package main

import (
	"cloudfoundry.org/cf-k8s-api/apis"
	"cloudfoundry.org/cf-k8s-api/routes"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"

	. "cloudfoundry.org/cf-k8s-api/config"
)
const defaultConfigPath = "config.json"

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

	// Configure the RootV3 API Handler
	apiRootV3Handler := &apis.RootV3Handler{
		ServerURL: config.ServerURL,
	}
	apiRootHandler := &apis.RootHandler{
		ServerURL: config.ServerURL,
	}

	router := mux.NewRouter()
	// create API routes
	apiRoutes := routes.APIRoutes{
		//add API routes to handler
		RootV3Handler: apiRootV3Handler.RootV3GetHandler,
		RootHandler: apiRootHandler.RootGetHandler,
	}
	// Call RegisterRoutes to register all the routes in APIRoutes
	apiRoutes.RegisterRoutes(router)

	portString := fmt.Sprintf(":%v", config.ServerPort)
	log.Fatal(http.ListenAndServe(portString, router))

}