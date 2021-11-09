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

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
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
	configPath, found := os.LookupEnv("CONFIG")
	if !found {
		panic("CONFIG must be set")
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
	privilegedK8sClient, err := repositories.BuildK8sClient(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create privileged k8s client: %v", err))
	}

	serverURL, err := url.Parse(config.ServerURL)
	if err != nil {
		panic(fmt.Sprintf("could not parse server URL: %v", err))
	}
	scaleProcessAction := actions.NewScaleProcess(new(repositories.ProcessRepository))
	scaleAppProcessAction := actions.NewScaleAppProcess(new(repositories.AppRepo), new(repositories.ProcessRepository), scaleProcessAction.Invoke)
	createAppAction := actions.NewCreateApp(new(repositories.AppRepo))

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
			new(repositories.AppRepo),
			new(repositories.DropletRepo),
			new(repositories.ProcessRepository),
			new(repositories.RouteRepo),
			new(repositories.DomainRepo),
			scaleAppProcessAction.Invoke,
			createAppAction.Invoke,
			repositories.BuildCRClient,
			k8sClientConfig,
		),
		apis.NewRouteHandler(
			ctrl.Log.WithName("RouteHandler"),
			*serverURL,
			new(repositories.RouteRepo),
			new(repositories.DomainRepo),
			new(repositories.AppRepo),
			repositories.BuildCRClient,
			k8sClientConfig,
		),
		apis.NewPackageHandler(
			ctrl.Log.WithName("PackageHandler"),
			*serverURL,
			new(repositories.PackageRepo),
			new(repositories.AppRepo),
			repositories.BuildCRClient,
			repositories.UploadSourceImage,
			newRegistryAuthBuilder(privilegedK8sClient, config),
			k8sClientConfig,
			config.PackageRegistryBase,
			config.PackageRegistrySecretName,
		),
		apis.NewBuildHandler(
			ctrl.Log.WithName("BuildHandler"),
			*serverURL,
			new(repositories.BuildRepo),
			new(repositories.PackageRepo),
			repositories.BuildCRClient,
			k8sClientConfig,
		),
		apis.NewDropletHandler(
			ctrl.Log.WithName("DropletHandler"),
			*serverURL,
			new(repositories.DropletRepo),
			repositories.BuildCRClient,
			k8sClientConfig,
		),
		apis.NewProcessHandler(
			ctrl.Log.WithName("ProcessHandler"),
			*serverURL,
			new(repositories.ProcessRepository),
			scaleProcessAction.Invoke,
			repositories.BuildCRClient,
			k8sClientConfig,
		),
		apis.NewDomainHandler(
			ctrl.Log.WithName("DomainHandler"),
			*serverURL,
			new(repositories.DomainRepo),
			repositories.BuildCRClient,
			k8sClientConfig,
		),
		apis.NewJobHandler(
			ctrl.Log.WithName("JobHandler"),
			*serverURL,
		),

		wireOrgHandler(*serverURL, orgRepo, privilegedCRClient, config.AuthEnabled),
		apis.NewSpaceHandler(
			repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, createTimeout),
			*serverURL,
		),

		apis.NewSpaceManifestHandler(
			ctrl.Log.WithName("SpaceManifestHandler"),
			*serverURL,
			actions.NewApplyManifest(new(repositories.AppRepo), createAppAction.Invoke).Invoke,
			repositories.NewOrgRepo(config.RootNamespace, privilegedCRClient, createTimeout),
			repositories.BuildCRClient,
			k8sClientConfig,
		),

		apis.NewRoleHandler(*serverURL, repositories.NewRoleRepo(privilegedCRClient, authorization.NewOrg(privilegedCRClient), config.RoleMappings)),
	}

	router := mux.NewRouter()
	for _, handler := range handlers {
		handler.RegisterRoutes(router)
	}

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Fatal(http.ListenAndServe(portString, router))
}

func newRegistryAuthBuilder(privilegedK8sClient k8sclient.Interface, config *config.Config) func(ctx context.Context) (remote.Option, error) {
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

func wireOrgHandler(serverUrl url.URL, orgRepo *repositories.OrgRepo, client client.Client, authEnabled bool) *apis.OrgHandler {
	var orgRepoProvider apis.OrgRepositoryProvider = provider.NewPrivilegedOrg(orgRepo)
	if authEnabled {
		authNsProvider := authorization.NewOrg(client)
		tokenReviewer := authorization.NewTokenReviewer(client)
		certInspector := authorization.NewCertInspector()
		identityProvider := authorization.NewIdentityProvider(tokenReviewer, certInspector)
		orgRepoProvider = provider.NewOrg(orgRepo, authNsProvider, identityProvider)
	}

	return apis.NewOrgHandler(serverUrl, orgRepoProvider)
}
