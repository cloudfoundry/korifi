package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/pivotal/kpack/pkg/registry"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/gorilla/mux"
	"github.com/pivotal/kpack/pkg/dockercreds/k8sdockercreds"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

var createTimeout = time.Second * 30

func init() {
	utilruntime.Must(workloadsv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(networkingv1alpha1.AddToScheme(scheme.Scheme))
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

	var buildUserClient repositories.UserK8sClientFactory = repositories.NewPrivilegedClientFactory(k8sClientConfig)
	if config.AuthEnabled {
		buildUserClient = repositories.NewUnprivilegedClientFactory(k8sClientConfig)
	}

	serverURL, err := url.Parse(config.ServerURL)
	if err != nil {
		panic(fmt.Sprintf("could not parse server URL: %v", err))
	}
	scaleProcessAction := actions.NewScaleProcess(repositories.NewProcessRepo(privilegedCRClient))
	scaleAppProcessAction := actions.NewScaleAppProcess(
		repositories.NewAppRepo(privilegedCRClient, buildUserClient),
		repositories.NewProcessRepo(privilegedCRClient),
		scaleProcessAction.Invoke,
	)

	fetchProcessStatsAction := actions.NewFetchProcessStats(
		repositories.NewProcessRepo(privilegedCRClient),
		repositories.NewPodRepo(privilegedCRClient),
		repositories.NewAppRepo(privilegedCRClient, buildUserClient),
	)

	identityProvider := wireIdentityProvider(privilegedCRClient, k8sClientConfig)
	orgRepo := repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, createTimeout)
	handlers := []APIHandler{
		apis.NewRootV3Handler(config.ServerURL),
		apis.NewRootHandler(
			ctrl.Log.WithName("RootHandler"),
			config.ServerURL,
		),
		apis.NewResourceMatchesHandler(config.ServerURL),
		apis.NewAppHandler(
			ctrl.Log.WithName("AppHandler"),
			*serverURL,
			repositories.NewAppRepo(privilegedCRClient, buildUserClient),
			repositories.NewDropletRepo(privilegedCRClient),
			repositories.NewProcessRepo(privilegedCRClient),
			repositories.NewRouteRepo(privilegedCRClient),
			repositories.NewDomainRepo(privilegedCRClient),
			repositories.NewPodRepo(privilegedCRClient),
			scaleAppProcessAction.Invoke,
		),
		apis.NewRouteHandler(
			ctrl.Log.WithName("RouteHandler"),
			*serverURL,
			repositories.NewRouteRepo(privilegedCRClient),
			repositories.NewDomainRepo(privilegedCRClient),
			repositories.NewAppRepo(privilegedCRClient, buildUserClient),
		),
		apis.NewPackageHandler(
			ctrl.Log.WithName("PackageHandler"),
			*serverURL,
			repositories.NewPackageRepo(privilegedCRClient),
			repositories.NewAppRepo(privilegedCRClient, buildUserClient),
			repositories.UploadSourceImage,
			newRegistryAuthBuilder(privilegedK8sClient, config),
			config.PackageRegistryBase,
			config.PackageRegistrySecretName,
		),
		apis.NewBuildHandler(
			ctrl.Log.WithName("BuildHandler"),
			*serverURL,
			repositories.NewBuildRepo(privilegedCRClient),
			repositories.NewPackageRepo(privilegedCRClient),
		),
		apis.NewDropletHandler(
			ctrl.Log.WithName("DropletHandler"),
			*serverURL,
			repositories.NewDropletRepo(privilegedCRClient),
		),
		apis.NewProcessHandler(
			ctrl.Log.WithName("ProcessHandler"),
			*serverURL,
			repositories.NewProcessRepo(privilegedCRClient),
			fetchProcessStatsAction.Invoke,
			scaleProcessAction.Invoke,
		),
		apis.NewDomainHandler(
			ctrl.Log.WithName("DomainHandler"),
			*serverURL,
			repositories.NewDomainRepo(privilegedCRClient),
		),
		apis.NewJobHandler(
			ctrl.Log.WithName("JobHandler"),
			*serverURL,
		),
		apis.NewLogCacheHandler(),

		apis.NewOrgHandler(*serverURL, wireOrgRepoProvider(orgRepo, privilegedCRClient, config.AuthEnabled, identityProvider)),
		apis.NewSpaceHandler(*serverURL, wireSpaceRepoProvider(orgRepo, privilegedCRClient, config.AuthEnabled, identityProvider)),

		apis.NewSpaceManifestHandler(
			ctrl.Log.WithName("SpaceManifestHandler"),
			*serverURL,
			actions.NewApplyManifest(repositories.NewAppRepo(privilegedCRClient, buildUserClient), repositories.NewProcessRepo(privilegedCRClient)).Invoke,
			repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, createTimeout),
		),

		apis.NewRoleHandler(*serverURL, repositories.NewRoleRepo(privilegedCRClient, authorization.NewOrg(privilegedCRClient), config.RoleMappings)),

		apis.NewWhoAmI(identityProvider, *serverURL),
	}

	router := mux.NewRouter()
	for _, handler := range handlers {
		handler.RegisterRoutes(router)
	}

	authInfoParser := authorization.NewInfoParser()
	router.Use(apis.NewAuthenticationMiddleware(
		ctrl.Log.WithName("AuthenticationMiddleware"),
		authInfoParser,
		identityProvider,
	).Middleware)

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Println("Listening on ", portString)
	log.Fatal(http.ListenAndServe(portString, router))
}

func newRegistryAuthBuilder(privilegedK8sClient k8sclient.Interface, config *config.APIConfig) func(ctx context.Context) (remote.Option, error) {
	return func(ctx context.Context) (remote.Option, error) {
		keychainFactory, err := k8sdockercreds.NewSecretKeychainFactory(privilegedK8sClient)
		if err != nil {
			return nil, fmt.Errorf("error in k8sdockercreds.NewSecretKeychainFactory: %w", err)
		}
		keychain, err := keychainFactory.KeychainForSecretRef(ctx, registry.SecretRef{
			Namespace:        config.RootNamespace,
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: config.PackageRegistrySecretName}},
		})
		if err != nil {
			return nil, fmt.Errorf("error in keychainFactory.KeychainForSecretRef: %w", err)
		}

		return remote.WithAuthFromKeychain(keychain), nil
	}
}

func wireOrgRepoProvider(orgRepo *repositories.OrgRepo, client client.Client, authEnabled bool, identityProvider *authorization.IdentityProvider) apis.OrgRepositoryProvider {
	if !authEnabled {
		return provider.NewPrivilegedOrg(orgRepo)
	}

	authNsProvider := authorization.NewOrg(client)
	return provider.NewOrg(orgRepo, authNsProvider, identityProvider)
}

func wireSpaceRepoProvider(orgRepo *repositories.OrgRepo, client client.Client, authEnabled bool, identityProvider *authorization.IdentityProvider) apis.SpaceRepositoryProvider {
	if !authEnabled {
		return provider.NewPrivilegedSpace(orgRepo)
	}

	authNsProvider := authorization.NewOrg(client)
	return provider.NewSpace(orgRepo, authNsProvider, identityProvider)
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) *authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewIdentityProvider(tokenReviewer, certInspector)
}
