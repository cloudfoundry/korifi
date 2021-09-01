package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	. "code.cloudfoundry.org/cf-k8s-api/config"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"code.cloudfoundry.org/cf-k8s-api/routes"
	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes/scheme"
)

const defaultConfigPath = "config.json"

func init() {
	utilruntime.Must(workloadsv1alpha1.AddToScheme(scheme.Scheme))
}

func main() {
	configPath := os.Getenv("CONFIG")
	if configPath == "" {
		configPath = defaultConfigPath
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

	// Configure the RootV3 API Handler
	apiRootV3Handler := &apis.RootV3Handler{
		ServerURL: config.ServerURL,
	}
	apiRootHandler := &apis.RootHandler{
		ServerURL: config.ServerURL,
	}
	appsHandler := &apis.AppHandler{
		ServerURL: config.ServerURL,
		AppRepo:   &repositories.AppRepo{},
		Logger:    ctrl.Log.WithName("AppHandler"),
		K8sConfig: k8sClientConfig,
	}

	router := mux.NewRouter()
	// create API routes
	apiRoutes := routes.APIRoutes{
		//add API routes to handler
		RootV3Handler: apiRootV3Handler.RootV3GetHandler,
		RootHandler:   apiRootHandler.RootGetHandler,
		AppsHandler:   appsHandler.AppsGetHandler,
	}
	// Call RegisterRoutes to register all the routes in APIRoutes
	apiRoutes.RegisterRoutes(router)

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Fatal(http.ListenAndServe(portString, router))
}
