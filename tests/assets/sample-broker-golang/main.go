package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sample-broker/osbapi"
	"strings"
)

const (
	hardcodedUserName = "broker-user"
	hardcodedPassword = "broker-password"
)

func main() {
	http.HandleFunc("GET /", helloWorldHandler)
	http.HandleFunc("GET /v2/catalog", getCatalogHandler)
	http.HandleFunc("PUT /v2/service_instances/{id}", provisionServiceInstanceHandler)
	http.HandleFunc("DELETE /v2/service_instances/{id}", deprovisionServiceInstanceHandler)
	http.HandleFunc("GET /v2/service_instances/{id}/last_operation", serviceInstanceLastOperationHandler)
	http.HandleFunc("PUT /v2/service_instances/{instance_id}/service_bindings/{binding_id}", bindHandler)
	http.HandleFunc("GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation", serviceBindingLastOperationHandler)
	http.HandleFunc("GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}", getServiceBindingHandler)
	http.HandleFunc("DELETE /v2/service_instances/{instance_id}/service_bindings/{binding_id}", unbindHandler)

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

func getCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

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
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to marshal catalog: %v", err)
		return
	}

	fmt.Fprintln(w, string(catalogBytes))
}

func provisionServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	fmt.Fprintf(w, `{"operation":"provision-%s"}`, r.PathValue("id"))
}

func deprovisionServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	fmt.Fprintf(w, `{"operation":"deprovision-%s"}`, r.PathValue("id"))
}

func serviceInstanceLastOperationHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	fmt.Fprint(w, `{"state":"succeeded"}`)
}

func bindHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, `{
		"operation":"bind-operation"
	}`)
}

func serviceBindingLastOperationHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	fmt.Fprint(w, `{"state":"succeeded"}`)
}

func getServiceBindingHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	fmt.Fprint(w, `{"credentials":{
		"user":"my-user",
		"password":"my-password"
	}}`)
}

func unbindHandler(w http.ResponseWriter, r *http.Request) {
	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v", err)
		return
	}

	fmt.Fprintf(w, `{"operation":"unbind-%s"}`, r.PathValue("binding_id"))
}

func checkCredentials(_ http.ResponseWriter, r *http.Request) (int, error) {
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		return http.StatusUnauthorized, errors.New("Authorization request header not specified")
	}

	headerSplit := strings.Split(authHeader, " ")
	if len(headerSplit) != 2 {
		return http.StatusUnauthorized, errors.New("Could not parse Authorization request header")
	}

	if headerSplit[0] != "Basic" {
		return http.StatusUnauthorized, errors.New("Unsupported Authorization request header scheme. Only 'Basic' is supported")
	}

	credBytes, err := base64.StdEncoding.DecodeString(headerSplit[1])
	if err != nil {
		return http.StatusUnauthorized, errors.New("Failed to decode Authorization request header")
	}

	creds := strings.Split(string(credBytes), ":")
	if len(creds) != 2 {
		return http.StatusUnauthorized, errors.New("Failed to extract user credentials from Authorization request header")
	}

	username := creds[0]
	password := creds[1]

	if username != hardcodedUserName || password != hardcodedPassword {
		return http.StatusForbidden, fmt.Errorf("Incorrect credentials: user %q, password %q. Use %q as username and %q as password", username, password, hardcodedUserName, hardcodedPassword)
	}

	return -1, nil
}
