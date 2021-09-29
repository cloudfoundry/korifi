package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/config"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	"github.com/gorilla/mux"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

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

	k8sClientConfig := ctrl.GetConfigOrDie()

	zapOpts := zap.Options{
		// TODO: this needs to be configurable
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

	privilegedClient, err := repositories.BuildClient(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create privileged k8s client: %v", err))
	}

	handlers := []APIHandler{
		apis.NewRootV3Handler(config.ServerURL),
		apis.NewRootHandler(
			ctrl.Log.WithName("RootHandler"),
			config.ServerURL,
		),
		apis.NewResourceMatchesHandler(config.ServerURL),
		apis.NewAppHandler(
			ctrl.Log.WithName("AppHandler"),
			config.ServerURL,
			&repositories.AppRepo{},
			repositories.BuildClient,
			k8sClientConfig,
		),
		apis.NewRouteHandler(
			ctrl.Log.WithName("RouteHandler"),
			config.ServerURL,
			&repositories.RouteRepo{},
			&repositories.DomainRepo{},
			&repositories.AppRepo{},
			repositories.BuildClient,
			k8sClientConfig,
		),
		apis.NewPackageHandler(
			ctrl.Log.WithName("PackageHandler"),
			config.ServerURL,
			&repositories.PackageRepo{},
			&repositories.AppRepo{},
			repositories.BuildClient,
			k8sClientConfig,
		),
		apis.NewBuildHandler(
			ctrl.Log.WithName("BuildHandler"),
			config.ServerURL,
			&repositories.BuildRepo{},
			repositories.BuildClient,
			k8sClientConfig,
    ),
		apis.NewOrgHandler(
			repositories.NewOrgRepo(config.RootNamespace, privilegedClient),
			config.ServerURL,
		),
	}

	router := mux.NewRouter()
	for _, handler := range handlers {
		handler.RegisterRoutes(router)
	}

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Fatal(http.ListenAndServe(portString, router))
}
