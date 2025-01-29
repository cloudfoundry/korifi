package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"sample-broker/osbapi"
)

const (
	hardcodedUserName = "broker-user"
	hardcodedPassword = "broker-password"
)

var inProgressOperations sync.Map

func main() {
	http.HandleFunc("GET /", helloWorldHandler)
	http.HandleFunc("GET /v2/catalog", getCatalogHandler)

	http.HandleFunc("PUT /v2/service_instances/{id}", provisionServiceInstanceHandler)
	http.HandleFunc("DELETE /v2/service_instances/{id}", deprovisionServiceInstanceHandler)
	http.HandleFunc("GET /v2/service_instances/{id}/last_operation", getLastOperationHandler)

	http.HandleFunc("PUT /v2/service_instances/{instance_id}/service_bindings/{binding_id}", bindHandler)
	http.HandleFunc("DELETE /v2/service_instances/{instance_id}/service_bindings/{binding_id}", unbindHandler)
	http.HandleFunc("GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation", getLastOperationHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log(fmt.Sprintf("Listening on port %s", port))
	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}

func helloWorldHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	respond(w, http.StatusOK, "Hi, I'm the sample broker!")
}

func getCatalogHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if status, err := checkCredentials(w, r); err != nil {
		respond(w, status, fmt.Sprintf("Credentials check failed: %v", err))
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
				MaintenanceInfo: osbapi.MaintenanceInfo{
					Version: "1.2.3",
				},
			}},
		}},
	}

	catalogBytes, err := json.Marshal(catalog)
	if err != nil {
		log(fmt.Sprintf("failed to marshal catalog: %v", err))
		respond(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal catalog: %v", err))
		return
	}

	respond(w, http.StatusOK, string(catalogBytes))
}

func provisionServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if status, err := checkCredentials(w, r); err != nil {
		respond(w, status, fmt.Sprintf("Credentials check failed: %v", err))
		return
	}

	asyncOperation(w, fmt.Sprintf("provision-%s", r.PathValue("id")), "{}")
}

func deprovisionServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v\n", err)
		return
	}

	asyncOperation(w, fmt.Sprintf("deprovision-%s", r.PathValue("id")), "{}")
}

func bindHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v\n", err)
		return
	}

	asyncOperation(w, fmt.Sprintf("bind-%s-%s", r.PathValue("instance_id"), r.PathValue("binding_id")), `{
		"credentials": {
			"username": "binding-user",
			"password": "binding-password"
		}
	}`)
}

func unbindHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v\n", err)
		return
	}

	asyncOperation(w, fmt.Sprintf("unbind-%s-%s", r.PathValue("instance_id"), r.PathValue("binding_id")), "{}")
}

func getLastOperationHandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if status, err := checkCredentials(w, r); err != nil {
		w.WriteHeader(status)
		fmt.Fprintf(w, "Credentials check failed: %v\n", err)
		return
	}

	operation, err := getOperation(r)
	if err != nil {
		log(fmt.Sprintf("failed to get operation: %v\n", err))
		respond(w, http.StatusInternalServerError, fmt.Sprintf("failed to get operation: %v\n", err))
		return
	}

	isDone := inProgressOperations.CompareAndSwap(operation, true, false)
	if isDone {
		log(fmt.Sprintf("operation %q succeeds", operation))
		respond(w, http.StatusOK, `{"state":"succeeded"}`)
		return
	}

	log(fmt.Sprintf("operation %q is in progress", operation))
	respond(w, http.StatusOK, `{"state":"in progress"}`)
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

func asyncOperation(w http.ResponseWriter, operationName string, asyncResultBody string) {
	inProgress, _ := inProgressOperations.LoadOrStore(operationName, true)
	if inProgress.(bool) {
		log(fmt.Sprintf("operation %q in progress", operationName))
		respond(w, http.StatusAccepted, fmt.Sprintf(`{
			"operation":"%s"
		}`, operationName))
		return
	}

	inProgressOperations.Delete(operationName)
	log(fmt.Sprintf("operation %q is done", operationName))
	respond(w, http.StatusOK, asyncResultBody)
}

func getOperation(r *http.Request) (string, error) {
	operation := r.URL.Query().Get("operation")
	if operation == "" {
		return "", fmt.Errorf("last operation request %q body does not contain operation query parameter", r.URL)
	}

	return operation, nil
}

func logRequest(r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log(fmt.Sprintf("failed to read request %s %v body: %v", r.Method, r.URL, err))
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	log(fmt.Sprintf("%s %v\nBody: %s", r.Method, r.URL, string(bodyBytes)))
}

func log(s string) {
	fmt.Printf("%s\n", s)
}

func respond(w http.ResponseWriter, statusCode int, responseContent string) {
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, "%s\n", responseContent)
}
