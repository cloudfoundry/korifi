package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	reporegistry "code.cloudfoundry.org/cf-k8s-controllers/api/repositories/registry"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/util/cache"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
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

	var userClientFactory repositories.UserK8sClientFactory = repositories.NewPrivilegedClientFactory(k8sClientConfig)
	if config.AuthEnabled {
		userClientFactory = repositories.NewUnprivilegedClientFactory(k8sClientConfig)
	}

	identityProvider := wireIdentityProvider(privilegedCRClient, k8sClientConfig)
	cachingIdentityProvider := authorization.NewCachingIdentityProvider(identityProvider, cache.NewExpiring())
	nsPermissions := authorization.NewNamespacePermissions(privilegedCRClient, cachingIdentityProvider, config.RootNamespace)

	serverURL, err := url.Parse(config.ServerURL)
	if err != nil {
		panic(fmt.Sprintf("could not parse server URL: %v", err))
	}

	orgRepo := repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, userClientFactory, nsPermissions, createTimeout, config.AuthEnabled)
	appRepo := repositories.NewAppRepo(privilegedCRClient, userClientFactory, nsPermissions)
	processRepo := repositories.NewProcessRepo(privilegedCRClient, userClientFactory)
	podRepo := repositories.NewPodRepo(userClientFactory)
	dropletRepo := repositories.NewDropletRepo(privilegedCRClient, userClientFactory)
	routeRepo := repositories.NewRouteRepo(privilegedCRClient, userClientFactory)
	domainRepo := repositories.NewDomainRepo(privilegedCRClient, userClientFactory)
	buildRepo := repositories.NewBuildRepo(privilegedCRClient, userClientFactory)
	packageRepo := repositories.NewPackageRepo(privilegedCRClient, userClientFactory)
	serviceInstanceRepo := repositories.NewServiceInstanceRepo(userClientFactory, nsPermissions)
	buildpackRepo := repositories.NewBuildpackRepository(userClientFactory)
	roleRepo := repositories.NewRoleRepo(
		privilegedCRClient,
		userClientFactory,
		authorization.NewNamespacePermissions(
			privilegedCRClient,
			cachingIdentityProvider,
			config.RootNamespace,
		),
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
		apis.NewResourceMatchesHandler(config.ServerURL),
		apis.NewAppHandler(
			ctrl.Log.WithName("AppHandler"),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			podRepo,
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

		apis.NewOrgHandler(*serverURL, orgRepo, decoderValidator),

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

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Println("Listening on ", portString)
	log.Fatal(http.ListenAndServe(portString, router))
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewCertTokenIdentityProvider(tokenReviewer, certInspector)
}
