package main

import (
	"context"
	"crypto/tls"
	"fmt"
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
	"code.cloudfoundry.org/korifi/api/middleware"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	"code.cloudfoundry.org/korifi/api/repositories/registry"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	toolsregistry "code.cloudfoundry.org/korifi/tools/registry"

	"github.com/google/go-containerregistry/pkg/v1/remote"
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
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var createTimeout = time.Second * 120

func init() {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme.Scheme))
	utilruntime.Must(metricsv1beta1.AddToScheme(scheme.Scheme))
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

	zapOpts := zap.Options{
		// TODO: this needs to be configurable
		Development: false,
		TimeEncoder: zapcore.RFC3339NanoTimeEncoder,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	klog.SetLogger(ctrl.Log)

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

	orgRepo := repositories.NewOrgRepo(
		config.RootNamespace,
		privilegedCRClient,
		userClientFactory,
		nsPermissions,
		createTimeout,
	)
	spaceRepo := repositories.NewSpaceRepo(
		namespaceRetriever,
		orgRepo,
		userClientFactory,
		nsPermissions,
		createTimeout,
	)
	processRepo := repositories.NewProcessRepo(
		namespaceRetriever,
		userClientFactory,
		nsPermissions,
	)
	podRepo := repositories.NewPodRepo(
		userClientFactory,
	)
	cfAppConditionAwaiter := conditions.NewConditionAwaiter[*korifiv1alpha1.CFApp,
		korifiv1alpha1.CFAppList](createTimeout)
	appRepo := repositories.NewAppRepo(
		namespaceRetriever,
		userClientFactory,
		nsPermissions,
		cfAppConditionAwaiter,
	)
	dropletRepo := repositories.NewDropletRepo(
		userClientFactory,
		namespaceRetriever,
		nsPermissions,
	)
	routeRepo := repositories.NewRouteRepo(
		namespaceRetriever,
		userClientFactory,
		nsPermissions,
	)
	domainRepo := repositories.NewDomainRepo(
		userClientFactory,
		namespaceRetriever,
		config.RootNamespace,
	)
	buildRepo := repositories.NewBuildRepo(
		namespaceRetriever,
		userClientFactory,
	)
	packageRepo := repositories.NewPackageRepo(
		userClientFactory,
		namespaceRetriever,
		nsPermissions,
		toolsregistry.NewRepositoryCreator(config.ContainerRegistryType),
		config.ContainerRepositoryPrefix,
	)
	serviceInstanceRepo := repositories.NewServiceInstanceRepo(
		namespaceRetriever,
		userClientFactory,
		nsPermissions,
	)
	serviceBindingRepo := repositories.NewServiceBindingRepo(
		namespaceRetriever,
		userClientFactory,
		nsPermissions,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFServiceBinding, korifiv1alpha1.CFServiceBindingList](createTimeout),
	)
	buildpackRepo := repositories.NewBuildpackRepository(config.BuilderName,
		userClientFactory,
		config.RootNamespace,
	)
	roleRepo := repositories.NewRoleRepo(
		userClientFactory,
		spaceRepo,
		authorization.NewNamespacePermissions(privilegedCRClient, cachingIdentityProvider),
		authorization.NewNamespacePermissions(privilegedCRClient, cachingIdentityProvider),
		config.RootNamespace,
		config.RoleMappings,
		namespaceRetriever,
	)
	imageRepo := repositories.NewImageRepository(
		privilegedK8sClient,
		userClientFactory,
		config.RootNamespace,
		config.PackageRegistrySecretName,
		registry.NewImageBuilder(),
		registry.NewImagePusher(remote.Write),
	)
	taskRepo := repositories.NewTaskRepo(
		userClientFactory,
		namespaceRetriever,
		nsPermissions,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList](createTimeout),
	)
	metricsRepo := repositories.NewMetricsRepo(userClientFactory)

	processStats := actions.NewProcessStats(processRepo, appRepo, metricsRepo)
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

	routerBuilder := routing.NewRouterBuilder()
	routerBuilder.UseMiddleware(
		middleware.Correlation(ctrl.Log),
		middleware.CFCliVersion,
		middleware.HTTPLogging,
	)

	authInfoParser := authorization.NewInfoParser()
	routerBuilder.UseAuthMiddleware(
		middleware.Authentication(
			authInfoParser,
			cachingIdentityProvider,
		),
		middleware.CFUser(
			privilegedCRClient,
			cachingIdentityProvider,
			config.RootNamespace,
			cache.NewExpiring(),
		),
	)

	apiHandlers := []routing.Routable{
		handlers.NewRootV3(config.ServerURL),
		handlers.NewRoot(
			config.ServerURL,
		),
		handlers.NewResourceMatches(),
		handlers.NewApp(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			decoderValidator,
		),
		handlers.NewRoute(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			decoderValidator,
		),
		handlers.NewServiceRouteBinding(
			*serverURL,
		),
		handlers.NewPackage(
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			imageRepo,
			decoderValidator,
			config.PackageRegistrySecretName,
		),
		handlers.NewBuild(
			*serverURL,
			buildRepo,
			packageRepo,
			appRepo,
			decoderValidator,
		),
		handlers.NewDroplet(
			*serverURL,
			dropletRepo,
		),
		handlers.NewProcess(
			*serverURL,
			processRepo,
			processStats,
			decoderValidator,
		),
		handlers.NewDomain(
			*serverURL,
			decoderValidator,
			domainRepo,
		),
		handlers.NewJob(
			*serverURL,
		),
		handlers.NewLogCache(
			appRepo,
			buildRepo,
			appLogs,
		),
		handlers.NewOrg(
			*serverURL,
			orgRepo,
			domainRepo,
			decoderValidator,
			config.GetUserCertificateDuration(),
		),
		handlers.NewSpace(
			*serverURL,
			spaceRepo,
			decoderValidator,
		),
		handlers.NewSpaceManifest(
			*serverURL,
			manifest,
			spaceRepo,
			decoderValidator,
		),
		handlers.NewRole(
			*serverURL,
			roleRepo,
			decoderValidator,
		),
		handlers.NewWhoAmI(cachingIdentityProvider, *serverURL),
		handlers.NewUser(*serverURL),
		handlers.NewBuildpack(
			*serverURL,
			buildpackRepo,
		),
		handlers.NewServiceInstance(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			decoderValidator,
		),
		handlers.NewServiceBinding(
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		),
		handlers.NewTask(
			*serverURL,
			appRepo,
			taskRepo,
			decoderValidator,
		),
		handlers.NewOAuth(
			*serverURL,
		),
	}
	for _, handler := range apiHandlers {
		routerBuilder.LoadRoutes(handler)
	}

	portString := fmt.Sprintf(":%v", config.InternalPort)
	tlsPath, tlsFound := os.LookupEnv("TLSCONFIG")

	srv := &http.Server{
		Addr:              portString,
		Handler:           routerBuilder.Build(),
		IdleTimeout:       time.Duration(config.IdleTimeout * int(time.Second)),
		ReadTimeout:       time.Duration(config.ReadTimeout * int(time.Second)),
		ReadHeaderTimeout: time.Duration(config.ReadHeaderTimeout * int(time.Second)),
		WriteTimeout:      time.Duration(config.WriteTimeout * int(time.Second)),
		ErrorLog:          log.New(&tools.LogrWriter{Logger: ctrl.Log, Message: "HTTP server error"}, "", 0),
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
