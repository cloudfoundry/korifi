package main

import (
	"fmt"
	"k8s.io/client-go/rest"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/apis"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	reporegistry "code.cloudfoundry.org/korifi/api/repositories/registry"
	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/gorilla/mux"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/util/cache"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var createTimeout = time.Second * 120

func init() {
	utilruntime.Must(v1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme.Scheme))
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
	buildpackRepo := repositories.NewBuildpackRepository(userClientFactory)
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
	readAppLogsAction := actions.NewReadAppLogs(appRepo, buildRepo, podRepo)

	decoderValidator, err := apis.NewDefaultDecoderValidator()
	if err != nil {
		panic(fmt.Sprintf("could not wire validator: %v", err))
	}

	handlers := []APIHandler{
		apis.NewRootV3Handler(config.ServerURL),
		apis.NewRootHandler(
			config.ServerURL,
		),
		apis.NewResourceMatchesHandler(),
		apis.NewAppHandler(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			scaleAppProcessAction.Invoke,
			decoderValidator,
		),
		apis.NewRouteHandler(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			decoderValidator,
		),
		apis.NewServiceRouteBindingHandler(
			*serverURL,
		),
		apis.NewPackageHandler(
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
			*serverURL,
			buildRepo,
			packageRepo,
			decoderValidator,
		),
		apis.NewDropletHandler(
			*serverURL,
			dropletRepo,
		),
		apis.NewProcessHandler(
			*serverURL,
			processRepo,
			fetchProcessStatsAction.Invoke,
			scaleProcessAction.Invoke,
			decoderValidator,
		),
		apis.NewDomainHandler(
			*serverURL,
			domainRepo,
		),
		apis.NewJobHandler(
			*serverURL,
		),
		apis.NewLogCacheHandler(
			appRepo,
			buildRepo,
			readAppLogsAction.Invoke,
		),
		apis.NewOrgHandler(
			*serverURL,
			orgRepo,
			domainRepo,
			decoderValidator,
			config.GetUserCertificateDuration(),
		),

		apis.NewSpaceHandler(
			*serverURL,
			config.PackageRegistrySecretName,
			spaceRepo,
			decoderValidator,
		),

		apis.NewSpaceManifestHandler(
			*serverURL,
			config.DefaultDomainName,
			applyManifestAction,
			spaceRepo,
			decoderValidator,
		),

		apis.NewRoleHandler(
			*serverURL,
			roleRepo,
			decoderValidator,
		),

		apis.NewWhoAmI(cachingIdentityProvider, *serverURL),

		apis.NewBuildpackHandler(
			*serverURL,
			buildpackRepo,
			config.ClusterBuilderName,
		),

		apis.NewServiceInstanceHandler(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			decoderValidator,
		),

		apis.NewServiceBindingHandler(
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
	router.Use(
		apis.NewCorrelationIDMiddleware().Middleware,
		apis.NewAuthenticationMiddleware(
			authInfoParser,
			cachingIdentityProvider,
		).Middleware,
	)

	portString := fmt.Sprintf(":%v", config.InternalPort)
	tlsPath, tlsFound := os.LookupEnv("TLSCONFIG")
	if tlsFound {
		log.Println("Listening with TLS on ", portString)
		log.Fatal(http.ListenAndServeTLS(portString, path.Join(tlsPath, "tls.crt"), path.Join(tlsPath, "tls.key"), router))
	} else {
		log.Println("Listening without TLS on ", portString)
		log.Fatal(http.ListenAndServe(portString, router))
	}
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewCertTokenIdentityProvider(tokenReviewer, certInspector)
}
