package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/blendle/zapdriver"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	reporegistry "code.cloudfoundry.org/korifi/api/repositories/registry"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/gorilla/mux"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/util/cache"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var createTimeout = time.Second * 120

func init() {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme.Scheme))
}

type APIHandler interface {
	RegisterRoutes(router *mux.Router)
}

// logrWriter implements io.Writer and converts Write calls to logr.Logger.Error() calls
type logrWriter struct {
	Logger  logr.Logger
	Message string
}

func (w *logrWriter) Write(msg []byte) (int, error) {
	w.Logger.Error(errors.New(string(msg)), w.Message)
	return len(msg), nil
}

func newZapLogger(config *config.APIConfig) (logr.Logger, zap.AtomicLevel) {
	cfg := zapdriver.NewProductionConfig()
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder // we don't know why this is needed
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	cfg.Level.SetLevel(config.LogLevel)
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return zapr.NewLogger(logger), cfg.Level
}

func watchLoggingConfigOrDie(ctx context.Context, watcher *fsnotify.Watcher, configFilepath string, logger logr.Logger, atomicLevel zap.AtomicLevel) error {
	err := watcher.Add(configFilepath)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				// Channel is closed.
				if !ok {
					return
				}

				logger.V(1).Info("logger level config event", "event", event)

				// Only care about events which may modify the contents of the file.
				if !(isWrite(event) || isRemove(event) || isCreate(event)) {
					continue
				}

				// If the file was removed, re-add the watch.
				if isRemove(event) {
					if err := watcher.Add(event.Name); err != nil {
						logger.Error(err, "error re-watching file")
					}
				}

				config, err := config.LoadFromPath(configFilepath)
				if err != nil {
					logger.Error(err, "error reading config")
					continue
				}
				if atomicLevel.Level() != config.LogLevel {
					logger.Info("Updating logging level", "originalLevel", atomicLevel.Level(), "newLevel", config.LogLevel)
					atomicLevel.SetLevel(config.LogLevel)
				}
			case err, ok := <-watcher.Errors:
				// Channel is closed.
				if !ok {
					return
				}

				logger.Error(err, "logger level watch error")
			}
		}
	}()

	logger.Info("Starting to watch config file at "+configFilepath+" for logger level changes", "currentLevel", atomicLevel.Level())

	<-ctx.Done()

	logger.Info("Stopping log level config watcher")

	return watcher.Close()
}

func isWrite(event fsnotify.Event) bool {
	return event.Op&fsnotify.Write == fsnotify.Write
}

func isCreate(event fsnotify.Event) bool {
	return event.Op&fsnotify.Create == fsnotify.Create
}

func isRemove(event fsnotify.Event) bool {
	return event.Op&fsnotify.Remove == fsnotify.Remove
}

func main() {
	configPath, found := os.LookupEnv("APICONFIG")
	if !found {
		panic("APICONFIG must be set")
	}
	config, err := config.LoadFromPath(configPath)
	if err != nil {
		errorMessage := fmt.Sprintf("Config could not be read: %v", err)
		panic(errorMessage)
	}
	payloads.DefaultLifecycleConfig = config.DefaultLifecycleConfig
	k8sClientConfig := config.GenerateK8sClientConfig(ctrl.GetConfigOrDie())

	logger, atomicLevel := newZapLogger(config)
	ctrl.SetLogger(logger)
	klog.SetLogger(ctrl.Log)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(fmt.Sprintf("error creating file watcher: %v", err))
	}

	go func() {
		if err2 := watchLoggingConfigOrDie(context.Background(), watcher, configPath, ctrl.Log, atomicLevel); err2 != nil {
			ctrl.Log.Error(err, "error watching TLS")
			os.Exit(1)
		}
	}()

	privilegedCRClient, err := client.NewWithWatch(k8sClientConfig, client.Options{})
	if err != nil {
		panic(fmt.Sprintf("could not create privileged k8s client: %v", err))
	}
	privilegedK8sClient, err := k8sclient.NewForConfig(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create privileged k8s client: %v", err))
	}

	dynamicClient, err := dynamic.NewForConfig(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create dynamic k8s client: %v", err))
	}
	namespaceRetriever := repositories.NewNamespaceRetriever(dynamicClient)

	mapper, err := apiutil.NewDynamicRESTMapper(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create kubernetes REST mapper: %v", err))
	}

	userClientFactory := authorization.NewUnprivilegedClientFactory(k8sClientConfig, mapper, authorization.NewDefaultBackoff())

	identityProvider := wireIdentityProvider(privilegedCRClient, k8sClientConfig)
	cachingIdentityProvider := authorization.NewCachingIdentityProvider(identityProvider, cache.NewExpiring())
	nsPermissions := authorization.NewNamespacePermissions(privilegedCRClient, cachingIdentityProvider)

	serverURL, err := url.Parse(config.ServerURL)
	if err != nil {
		panic(fmt.Sprintf("could not parse server URL: %v", err))
	}

	metricsFetcherFunction, err := repositories.CreateMetricsFetcher(k8sClientConfig)
	if err != nil {
		panic(err)
	}
	orgRepo := repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, userClientFactory, nsPermissions, createTimeout)
	spaceRepo := repositories.NewSpaceRepo(namespaceRetriever, orgRepo, userClientFactory, nsPermissions, createTimeout)
	processRepo := repositories.NewProcessRepo(namespaceRetriever, userClientFactory, nsPermissions)
	podRepo := repositories.NewPodRepo(userClientFactory, metricsFetcherFunction)
	cfAppConditionAwaiter := conditions.NewConditionAwaiter[*korifiv1alpha1.CFApp, korifiv1alpha1.CFAppList](createTimeout)
	appRepo := repositories.NewAppRepo(namespaceRetriever, userClientFactory, nsPermissions, cfAppConditionAwaiter)
	dropletRepo := repositories.NewDropletRepo(userClientFactory, namespaceRetriever, nsPermissions)
	routeRepo := repositories.NewRouteRepo(namespaceRetriever, userClientFactory, nsPermissions)
	domainRepo := repositories.NewDomainRepo(userClientFactory, namespaceRetriever, config.RootNamespace)
	buildRepo := repositories.NewBuildRepo(namespaceRetriever, userClientFactory)
	packageRepo := repositories.NewPackageRepo(userClientFactory, namespaceRetriever, nsPermissions)
	serviceInstanceRepo := repositories.NewServiceInstanceRepo(namespaceRetriever, userClientFactory, nsPermissions)
	bindingConditionAwaiter := conditions.NewConditionAwaiter[*korifiv1alpha1.CFServiceBinding, korifiv1alpha1.CFServiceBindingList](createTimeout)
	serviceBindingRepo := repositories.NewServiceBindingRepo(namespaceRetriever, userClientFactory, nsPermissions, bindingConditionAwaiter)
	buildpackRepo := repositories.NewBuildpackRepository(config.BuilderName, userClientFactory, config.RootNamespace)
	roleRepo := repositories.NewRoleRepo(
		userClientFactory,
		spaceRepo,
		authorization.NewNamespacePermissions(privilegedCRClient, cachingIdentityProvider),
		config.RootNamespace,
		config.RoleMappings,
	)
	registryCAPath, found := os.LookupEnv("REGISTRY_CA_FILE")
	if !found {
		registryCAPath = ""
	}

	imageRepo := repositories.NewImageRepository(
		privilegedK8sClient,
		userClientFactory,
		config.RootNamespace,
		config.PackageRegistrySecretName,
		registryCAPath,
		reporegistry.NewImageBuilder(),
		reporegistry.NewImagePusher(remote.Write),
	)
	taskRepo := repositories.NewTaskRepo(
		userClientFactory,
		namespaceRetriever,
		nsPermissions,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList](createTimeout),
	)

	processScaler := actions.NewProcessScaler(appRepo, processRepo)
	processStats := actions.NewProcessStats(processRepo, podRepo, appRepo)
	manifest := actions.NewManifest(
		domainRepo,
		config.DefaultDomainName,
		manifest.NewStateCollector(appRepo, domainRepo, processRepo, routeRepo),
		manifest.NewNormalizer(config.DefaultDomainName),
		manifest.NewApplier(appRepo, domainRepo, processRepo, routeRepo),
	)
	appLogs := actions.NewAppLogs(appRepo, buildRepo, podRepo)

	decoderValidator, err := handlers.NewDefaultDecoderValidator()
	if err != nil {
		panic(fmt.Sprintf("could not wire validator: %v", err))
	}

	apiHandlers := []APIHandler{
		handlers.NewRootV3Handler(config.ServerURL),
		handlers.NewRootHandler(
			config.ServerURL,
		),
		handlers.NewResourceMatchesHandler(),
		handlers.NewAppHandler(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			processScaler,
			decoderValidator,
		),
		handlers.NewRouteHandler(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			decoderValidator,
		),
		handlers.NewServiceRouteBindingHandler(
			*serverURL,
		),
		handlers.NewPackageHandler(
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			imageRepo,
			decoderValidator,
			config.PackageRegistryBase,
			config.PackageRegistrySecretName,
		),
		handlers.NewBuildHandler(
			*serverURL,
			buildRepo,
			packageRepo,
			appRepo,
			decoderValidator,
		),
		handlers.NewDropletHandler(
			*serverURL,
			dropletRepo,
		),
		handlers.NewProcessHandler(
			*serverURL,
			processRepo,
			processStats,
			processScaler,
			decoderValidator,
		),
		handlers.NewDomainHandler(
			*serverURL,
			domainRepo,
		),
		handlers.NewJobHandler(
			*serverURL,
		),
		handlers.NewLogCacheHandler(
			appRepo,
			buildRepo,
			appLogs,
		),
		handlers.NewOrgHandler(
			*serverURL,
			orgRepo,
			domainRepo,
			decoderValidator,
			config.GetUserCertificateDuration(),
		),

		handlers.NewSpaceHandler(
			*serverURL,
			spaceRepo,
			decoderValidator,
		),

		handlers.NewSpaceManifestHandler(
			*serverURL,
			manifest,
			spaceRepo,
			decoderValidator,
		),

		handlers.NewRoleHandler(
			*serverURL,
			roleRepo,
			decoderValidator,
		),

		handlers.NewWhoAmI(cachingIdentityProvider, *serverURL),

		handlers.NewBuildpackHandler(
			*serverURL,
			buildpackRepo,
		),

		handlers.NewServiceInstanceHandler(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			decoderValidator,
		),

		handlers.NewServiceBindingHandler(
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		),

		handlers.NewTaskHandler(
			*serverURL,
			appRepo,
			taskRepo,
			decoderValidator,
		),

		handlers.NewOAuthToken(
			*serverURL,
		),
	}

	router := mux.NewRouter()
	for _, handler := range apiHandlers {
		handler.RegisterRoutes(router)
	}

	unauthenticatedEndpoints := handlers.NewUnauthenticatedEndpoints()
	authInfoParser := authorization.NewInfoParser()
	router.Use(
		handlers.NewCorrelationIDMiddleware().Middleware,
		handlers.NewCFCliVersionMiddleware().Middleware,
		handlers.NewHTTPLogging().Middleware,
		handlers.NewAuthenticationMiddleware(
			authInfoParser,
			cachingIdentityProvider,
			unauthenticatedEndpoints,
		).Middleware,
		handlers.NewCFUserMiddleware(
			privilegedCRClient,
			cachingIdentityProvider,
			config.RootNamespace,
			cache.NewExpiring(),
			unauthenticatedEndpoints,
		).Middleware,
	)

	portString := fmt.Sprintf(":%v", config.InternalPort)
	tlsPath, tlsFound := os.LookupEnv("TLSCONFIG")

	srv := &http.Server{
		Addr:              portString,
		Handler:           router,
		IdleTimeout:       time.Duration(config.IdleTimeout * int(time.Second)),
		ReadTimeout:       time.Duration(config.ReadTimeout * int(time.Second)),
		ReadHeaderTimeout: time.Duration(config.ReadHeaderTimeout * int(time.Second)),
		WriteTimeout:      time.Duration(config.WriteTimeout * int(time.Second)),
		ErrorLog:          log.New(&logrWriter{Logger: ctrl.Log, Message: "HTTP server error"}, "", 0),
	}

	if tlsFound {
		ctrl.Log.Info("Listening with TLS on " + portString)
		certPath := filepath.Join(tlsPath, "tls.crt")
		keyPath := filepath.Join(tlsPath, "tls.key")

		var certWatcher *certwatcher.CertWatcher
		certWatcher, err = certwatcher.New(certPath, keyPath)
		if err != nil {
			ctrl.Log.Error(err, "error creating TLS watcher")
			os.Exit(1)
		}

		go func() {
			if err2 := certWatcher.Start(context.Background()); err2 != nil {
				ctrl.Log.Error(err, "error watching TLS")
				os.Exit(1)
			}
		}()

		srv.TLSConfig = &tls.Config{
			NextProtos:     []string{"h2"},
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certWatcher.GetCertificate,
		}
		err = srv.ListenAndServeTLS("", "")
		if err != nil {
			ctrl.Log.Error(err, "error serving TLS")
			os.Exit(1)
		}
	} else {
		ctrl.Log.Info("Listening without TLS on " + portString)
		err := srv.ListenAndServe()
		if err != nil {
			ctrl.Log.Error(err, "error serving HTTP")
			os.Exit(1)
		}
	}
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewCertTokenIdentityProvider(tokenReviewer, certInspector)
}
