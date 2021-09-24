package apis_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestRootAPI(t *testing.T) {
	spec.Run(t, "GET / endpoint", testRootAPI, spec.Report(report.Terminal{}))
}

func testRootAPI(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var rr *httptest.ResponseRecorder

	it.Before(func() {
		req, err := http.NewRequest("GET", "/", nil)
		g.Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		router := mux.NewRouter()

		apiHandler := apis.RootHandler{
			ServerURL: defaultServerURL,
		}
		apiHandler.RegisterRoutes(router)

		router.ServeHTTP(rr, req)
	})

	it("returns status 200 OK", func() {
		g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
	})

	it("returns Content-Type as JSON in header", func() {
		contentTypeHeader := rr.Header().Get("Content-Type")
		g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
	})

	it("has a non-empty body", func() {
		g.Expect(rr.Body.Bytes()).NotTo(BeEmpty())
	})

	it("matches the expected response body format", func() {
		expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `"},"bits_service":null,"cloud_controller_v2":null,"cloud_controller_v3":{"href":"` + defaultServerURL + `/v3","meta":{"version":"3.90.0"}},"network_policy_v0":null,"network_policy_v1":null,"login":null,"uaa":null,"credhub":null,"routing":null,"logging":null,"log_cache":null,"log_stream":null,"app_ssh":null}}`
		g.Expect(rr.Body.String()).To(Equal(expectedBody), "Response body matches RootV3GetHandler response:")
	})
}
