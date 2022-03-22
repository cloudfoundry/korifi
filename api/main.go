package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	reporegistry "code.cloudfoundry.org/cf-k8s-controllers/api/repositories/registry"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/gorilla/mux"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/util/cache"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var createTimeout = time.Second * 120

func init() {
	utilruntime.Must(workloadsv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(networkingv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(servicesv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme.Scheme))
	utilruntime.Must(hnsv1alpha2.AddToScheme(scheme.Scheme))
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
	k8sClientConfig := ctrl.GetConfigOrDie()

	zapOpts := zap.Options{
		// TODO: this needs to be configurable
		Development: true,
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

	var userClientFactory repositories.UserK8sClientFactory = repositories.NewPrivilegedClientFactory(k8sClientConfig, mapper)
	if config.AuthEnabled {
		userClientFactory = repositories.NewUnprivilegedClientFactory(k8sClientConfig, mapper)
	}

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
	orgRepo := repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, userClientFactory, nsPermissions, createTimeout, config.AuthEnabled)
	appRepo := repositories.NewAppRepo(privilegedCRClient, namespaceRetriever, userClientFactory, nsPermissions)
	processRepo := repositories.NewProcessRepo(privilegedCRClient, namespaceRetriever, userClientFactory, nsPermissions)
	podRepo := repositories.NewPodRepo(userClientFactory, metricsFetcherFunction)
	dropletRepo := repositories.NewDropletRepo(privilegedCRClient, namespaceRetriever, userClientFactory, nsPermissions)
	routeRepo := repositories.NewRouteRepo(privilegedCRClient, namespaceRetriever, userClientFactory, nsPermissions)
	domainRepo := repositories.NewDomainRepo(config.RootNamespace, privilegedCRClient, namespaceRetriever, userClientFactory)
	buildRepo := repositories.NewBuildRepo(namespaceRetriever, userClientFactory)
	packageRepo := repositories.NewPackageRepo(privilegedCRClient, namespaceRetriever, userClientFactory, nsPermissions)
	serviceInstanceRepo := repositories.NewServiceInstanceRepo(namespaceRetriever, userClientFactory, nsPermissions)
	serviceBindingRepo := repositories.NewServiceBindingRepo(namespaceRetriever, userClientFactory, nsPermissions)
	buildpackRepo := repositories.NewBuildpackRepository(userClientFactory)
	roleRepo := repositories.NewRoleRepo(
		privilegedCRClient,
		userClientFactory,
		authorization.NewNamespacePermissions(
			privilegedCRClient,
			cachingIdentityProvider,
			config.RootNamespace,
		),
		config.RootNamespace,
		config.RoleMappings,
	)
	imageRepo := repositories.NewImageRepository(
		privilegedK8sClient,
		userClientFactory,
		config.RootNamespace,
		config.PackageRegistrySecretName,
		reporegistry.NewImageBuilder(),
		reporegistry.NewImagePusher(remote.Write),
	)

	scaleProcessAction := actions.NewScaleProcess(processRepo)
	scaleAppProcessAction := actions.NewScaleAppProcess(appRepo, processRepo, scaleProcessAction.Invoke)
	fetchProcessStatsAction := actions.NewFetchProcessStats(processRepo, podRepo, appRepo)
	applyManifestAction := actions.NewApplyManifest(
		appRepo,
		domainRepo,
		processRepo,
		routeRepo,
	).Invoke

	decoderValidator, err := apis.NewDefaultDecoderValidator()
	if err != nil {
		panic(fmt.Sprintf("could not wire validator: %v", err))
	}

	handlers := []APIHandler{
		apis.NewRootV3Handler(config.ServerURL),
		apis.NewRootHandler(
			config.ServerURL,
		),
		apis.NewResourceMatchesHandler(ctrl.Log.WithName("ResourceMatchesHandler")),
		apis.NewAppHandler(
			ctrl.Log.WithName("AppHandler"),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			orgRepo,
			scaleAppProcessAction.Invoke,
			decoderValidator,
		),
		apis.NewRouteHandler(
			ctrl.Log.WithName("RouteHandler"),
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			orgRepo,
			decoderValidator,
		),
		apis.NewServiceRouteBindingHandler(
			ctrl.Log.WithName("ServiceRouteBinding"),
			*serverURL,
		),
		apis.NewPackageHandler(
			ctrl.Log.WithName("PackageHandler"),
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			imageRepo,
			decoderValidator,
			config.PackageRegistryBase,
			config.PackageRegistrySecretName,
		),
		apis.NewBuildHandler(
			ctrl.Log.WithName("BuildHandler"),
			*serverURL,
			buildRepo,
			packageRepo,
			decoderValidator,
		),
		apis.NewDropletHandler(
			ctrl.Log.WithName("DropletHandler"),
			*serverURL,
			dropletRepo,
		),
		apis.NewProcessHandler(
			ctrl.Log.WithName("ProcessHandler"),
			*serverURL,
			processRepo,
			fetchProcessStatsAction.Invoke,
			scaleProcessAction.Invoke,
			decoderValidator,
		),
		apis.NewDomainHandler(
			ctrl.Log.WithName("DomainHandler"),
			*serverURL,
			domainRepo,
		),
		apis.NewJobHandler(
			ctrl.Log.WithName("JobHandler"),
			*serverURL,
		),
		apis.NewLogCacheHandler(),

		apis.NewOrgHandler(*serverURL, orgRepo, domainRepo, decoderValidator),

		apis.NewSpaceHandler(*serverURL, config.PackageRegistrySecretName, orgRepo, decoderValidator),

		apis.NewSpaceManifestHandler(
			ctrl.Log.WithName("SpaceManifestHandler"),
			*serverURL,
			config.DefaultDomainName,
			applyManifestAction,
			orgRepo,
			decoderValidator,
		),

		apis.NewRoleHandler(
			*serverURL,
			roleRepo,
			decoderValidator,
		),

		apis.NewWhoAmI(cachingIdentityProvider, *serverURL),

		apis.NewBuildpackHandler(
			ctrl.Log.WithName("BuildpackHandler"),
			*serverURL,
			buildpackRepo,
			config.ClusterBuilderName,
		),

		apis.NewServiceInstanceHandler(
			ctrl.Log.WithName("ServiceInstanceHandler"),
			*serverURL,
			serviceInstanceRepo,
			orgRepo,
			decoderValidator,
		),

		apis.NewServiceBindingHandler(
			ctrl.Log.WithName("ServiceBindingHandler"),
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		),
	}

	router := mux.NewRouter()
	for _, handler := range handlers {
		handler.RegisterRoutes(router)
	}

	authInfoParser := authorization.NewInfoParser()
	router.Use(apis.NewAuthenticationMiddleware(
		ctrl.Log.WithName("AuthenticationMiddleware"),
		authInfoParser,
		cachingIdentityProvider,
	).Middleware)

	portString := fmt.Sprintf(":%v", config.InternalPort)
	log.Println("Listening on ", portString)
	log.Fatal(http.ListenAndServe(portString, router))
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewCertTokenIdentityProvider(tokenReviewer, certInspector)
}
