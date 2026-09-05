package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.cloudfoundry.org/korifi/osb-service/pkg/broker"
)

func TestCatalogRequiresAndEchoesOSBHeaders(t *testing.T) {
	h := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/v2/catalog", nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("without version header: got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v2/catalog", nil)
	request.Header.Set("X-Broker-API-Version", "2.17")
	request.Header.Set("X-Broker-API-Request-Identity", "test-request")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("catalog: got %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Broker-API-Request-Identity"); got != "test-request" {
		t.Fatalf("request identity: got %q", got)
	}
	var catalog broker.CatalogResponse
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Services) != 1 || !catalog.Services[0].InstancesRetrievable || !catalog.Services[0].BindingsRetrievable {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
}

func TestLifecycle(t *testing.T) {
	h := testHandler(t)
	instance := "/v2/service_instances/instance-1"
	do(t, h, http.MethodPut, instance, map[string]any{"service_id": broker.ServiceID, "plan_id": broker.PlanID}, http.StatusCreated)
	do(t, h, http.MethodPut, instance, map[string]any{"service_id": broker.ServiceID, "plan_id": broker.PlanID}, http.StatusOK)
	do(t, h, http.MethodGet, instance, nil, http.StatusOK)
	binding := instance + "/service_bindings/binding-1"
	do(t, h, http.MethodPut, binding, map[string]any{"service_id": broker.ServiceID, "plan_id": broker.PlanID}, http.StatusCreated)
	do(t, h, http.MethodGet, binding, nil, http.StatusOK)
	do(t, h, http.MethodDelete, binding+"?service_id="+broker.ServiceID+"&plan_id="+broker.PlanID, nil, http.StatusOK)
	do(t, h, http.MethodDelete, instance+"?service_id="+broker.ServiceID+"&plan_id="+broker.PlanID, nil, http.StatusOK)
	do(t, h, http.MethodGet, instance, nil, http.StatusNotFound)
}

func TestIgnoresVendorExtensionFields(t *testing.T) {
	h := testHandler(t)
	do(t, h, http.MethodPut, "/v2/service_instances/i", map[string]any{"service_id": broker.ServiceID, "plan_id": broker.PlanID, "vendor_extension": true}, http.StatusCreated)
}

func TestHealthzSkipsBasicAuth(t *testing.T) {
	h := basicAuth("broker", "secret", testHandler(t))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("healthz: got %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v2/catalog", nil)
	request.Header.Set("X-Broker-API-Version", "2.17")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("catalog without auth: got %d", response.Code)
	}
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	logic, err := broker.NewBusinessLogic(broker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return newHandler(logic)
}
func do(t *testing.T, h http.Handler, method, target string, body any, want int) {
	t.Helper()
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, target, &data)
	r.Header.Set("X-Broker-API-Version", "2.17")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: got %d, want %d: %s", method, target, w.Code, want, w.Body.String())
	}
}
