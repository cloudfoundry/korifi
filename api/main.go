package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	reporegistry "code.cloudfoundry.org/korifi/api/repositories/registry"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/gorilla/mux"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"k8s.io/apimachinery/pkg/util/cache"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var createTimeout = time.Second * 120

func init() {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme.Scheme))
}

type APIHandler interface {
	RegisterRoutes(router *mux.Router)
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
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

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
	namespaceRetriever := repositories.NewNamespaceRetriver(dynamicClient)

	mapper, err := apiutil.NewDynamicRESTMapper(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create kubernetes REST mapper: %v", err))
	}

	userClientFactory := authorization.NewUnprivilegedClientFactory(k8sClientConfig, mapper, authorization.NewDefaultBackoff())

	identityProvider := wireIdentityProvider(privilegedCRClient, k8sClientConfig)
	cachingIdentityProvider := authorization.NewCachingIdentityProvider(identityProvider, cache.NewExpiring())
	nsPermissions := authorization.NewNamespacePermissions(privilegedCRClient, cachingIdentityProvider, config.RootNamespace)

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
	appRepo := repositories.NewAppRepo(namespaceRetriever, userClientFactory, nsPermissions)
	dropletRepo := repositories.NewDropletRepo(userClientFactory, namespaceRetriever, nsPermissions)
	routeRepo := repositories.NewRouteRepo(namespaceRetriever, userClientFactory, nsPermissions)
	domainRepo := repositories.NewDomainRepo(userClientFactory, namespaceRetriever, config.RootNamespace)
	buildRepo := repositories.NewBuildRepo(namespaceRetriever, userClientFactory)
	packageRepo := repositories.NewPackageRepo(userClientFactory, namespaceRetriever, nsPermissions)
	serviceInstanceRepo := repositories.NewServiceInstanceRepo(namespaceRetriever, userClientFactory, nsPermissions)
	serviceBindingRepo := repositories.NewServiceBindingRepo(namespaceRetriever, userClientFactory, nsPermissions)
	buildpackRepo := repositories.NewBuildpackRepository(userClientFactory, config.RootNamespace)
	roleRepo := repositories.NewRoleRepo(
		userClientFactory,
		spaceRepo,
		authorization.NewNamespacePermissions(
			privilegedCRClient,
			cachingIdentityProvider,
			config.RootNamespace,
		),
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
	taskRepo := repositories.NewTaskRepo(userClientFactory, namespaceRetriever, nsPermissions, createTimeout)

	processScaler := actions.NewProcessScaler(appRepo, processRepo)
	processStats := actions.NewProcessStats(processRepo, podRepo, appRepo)
	manifest := actions.NewManifest(
		appRepo,
		domainRepo,
		processRepo,
		routeRepo,
		config.DefaultDomainName,
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
			config.PackageRegistrySecretName,
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
	}

	router := mux.NewRouter()
	for _, handler := range apiHandlers {
		handler.RegisterRoutes(router)
	}

	authInfoParser := authorization.NewInfoParser()
	router.Use(
		handlers.NewCorrelationIDMiddleware().Middleware,
		handlers.NewAuthenticationMiddleware(
			authInfoParser,
			cachingIdentityProvider,
		).Middleware,
	)

	tp, err := jaegerTracerProvider("jaeger:4317")
	if err != nil {
		log.Fatal(err)
	}
	otel.SetTracerProvider(tp)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cleanly shutdown and flush telemetry when the application exits.
	defer func(ctx context.Context) {
		// Do not make the application hang when it is shutdown.
		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

	portString := fmt.Sprintf(":%v", config.InternalPort)
	tlsPath, tlsFound := os.LookupEnv("TLSCONFIG")
	if tlsFound {
		log.Println("Listening with TLS on ", portString)
		log.Fatal(http.ListenAndServeTLS(portString, path.Join(tlsPath, "tls.crt"), path.Join(tlsPath, "tls.key"), router))
	} else {
		log.Println("Listening without TLS on ", portString)
		log.Fatal(http.ListenAndServe(portString, otelhttp.NewHandler(router, "server")))
	}
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewCertTokenIdentityProvider(tokenReviewer, certInspector)
}

func jaegerTracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("korifi-api"),
			attribute.String("environment", "kieron"),
			attribute.Int64("ID", 1),
		)),
	)
	return tp, nil
}
