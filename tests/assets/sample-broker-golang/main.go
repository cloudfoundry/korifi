package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"sample-broker/osbapi"
)

func main() {
	http.HandleFunc("/", helloWorldHandler)
	http.HandleFunc("/v2/catalog", getCatalogHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Listening on port %s\n", port)
	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}

func helloWorldHandler(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, "Hi, I'm the sample broker!")
}

func getCatalogHandler(w http.ResponseWriter, _ *http.Request) {
	catalog := osbapi.Catalog{
		Services: []osbapi.Service{{
			Name:        "sample-service",
			Id:          "edfd6e50-aa59-4688-b5bf-b21e2ab27cdb",
			Description: "A sample service that does nothing",
			Plans: []osbapi.Plan{{
				Id:          "ebf1c1df-fefb-479b-9231-ddf700a37b58",
				Name:        "sample",
				Description: "Sample plan",
				Free:        true,
				Bindable:    true,
			}},
		}},
	}

	catalogBytes, err := json.Marshal(catalog)
	if err != nil {
		fmt.Fprintf(w, "Failed to marshal catalog: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, string(catalogBytes))
}
