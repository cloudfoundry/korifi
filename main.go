package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	. "code.cloudfoundry.org/cf-k8s-api/config"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	utilruntime.Must(workloadsv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(networkingv1alpha1.AddToScheme(scheme.Scheme))
}

type APIHandler interface {
	RegisterRoutes(router *mux.Router)
}

func main() {
	configPath, found := os.LookupEnv("CONFIG")
	if !found {
		panic("CONFIG must be set")
	}

	config, err := LoadConfigFromPath(configPath)
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

	handlers := []APIHandler{
		&apis.RootV3Handler{
			ServerURL: config.ServerURL,
		},
		&apis.RootHandler{
			ServerURL: config.ServerURL,
		},
		&apis.ResourceMatchesHandler{
			ServerURL: config.ServerURL,
		},
		&apis.AppHandler{
			ServerURL:   config.ServerURL,
			AppRepo:     &repositories.AppRepo{},
			Logger:      ctrl.Log.WithName("AppHandler"),
			K8sConfig:   k8sClientConfig,
			BuildClient: repositories.BuildClient,
		},
		&apis.RouteHandler{
			ServerURL:   config.ServerURL,
			RouteRepo:   &repositories.RouteRepo{},
			DomainRepo:  &repositories.DomainRepo{},
			AppRepo:     &repositories.AppRepo{},
			Logger:      ctrl.Log.WithName("RouteHandler"),
			K8sConfig:   k8sClientConfig,
			BuildClient: repositories.BuildClient,
		},
		&apis.PackageHandler{
			ServerURL:   config.ServerURL,
			PackageRepo: &repositories.PackageRepo{},
			AppRepo:     &repositories.AppRepo{},
			K8sConfig:   k8sClientConfig,
			Logger:      ctrl.Log.WithName("PackageHandler"),
			BuildClient: repositories.BuildClient,
		},
	}

	router := mux.NewRouter()
	for _, handler := range handlers {
		handler.RegisterRoutes(router)
	}

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Fatal(http.ListenAndServe(portString, router))
}
