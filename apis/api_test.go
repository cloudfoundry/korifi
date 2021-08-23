package apis_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

const (
	jsonHeader       = "application/json"
	defaultServerURL = "https://api.example.org"
)

func TestAPI(t *testing.T) {
	spec.Run(t, "object", testRootV3API, spec.Report(report.Terminal{}))
	spec.Run(t, "object", testRootAPI, spec.Report(report.Terminal{}))
}

func testRootV3API(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect

	when("the v3 GET endpoint returns successfully", func() {
		var rr *httptest.ResponseRecorder
		it.Before(func() {
			// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
			// pass 'nil' as the third parameter.
			req, err := http.NewRequest("GET", "/v3", nil)
			Expect(err).NotTo(HaveOccurred())

			// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
			rr = httptest.NewRecorder()
			apiHandler := apis.RootV3Handler{
				ServerURL: defaultServerURL,
			}

			handler := http.HandlerFunc(apiHandler.RootV3GetHandler)

			// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
			// directly and pass in our Request and ResponseRecorder.
			handler.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			httpStatus := rr.Code
			Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})
		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})
		it("matches the expected response body format", func() {
			expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `/v3"}}}`
			Expect(rr.Body.String()).Should(Equal(expectedBody), "Response body matches RootV3GetHandler response:")
		})

	})
}

func testRootAPI(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect

	when("the root GET endpoint returns successfully", func() {
		var rr *httptest.ResponseRecorder
		it.Before(func() {
			// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
			// pass 'nil' as the third parameter.
			req, err := http.NewRequest("GET", "/", nil)
			Expect(err).NotTo(HaveOccurred())

			// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
			rr = httptest.NewRecorder()
			apiHandler := apis.RootHandler{
				ServerURL: defaultServerURL,
			}

			handler := http.HandlerFunc(apiHandler.RootGetHandler)

			// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
			// directly and pass in our Request and ResponseRecorder.
			handler.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			httpStatus := rr.Code
			Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})
		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})
		it("has a non-empty body", func() {
			responseBody := rr.Body.Bytes()
			Expect(responseBody).NotTo(BeEmpty())
		})
		it("matches the expected response body format", func() {
			expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `"},"bits_service":null,"cloud_controller_v2":null,"cloud_controller_v3":{"href":"` + defaultServerURL + `/v3","meta":{"version":"3.90.0"}},"network_policy_v0":null,"network_policy_v1":null,"login":null,"uaa":null,"credhub":null,"routing":null,"logging":null,"log_cache":null,"log_stream":null,"app_ssh":null}}`
			Expect(rr.Body.String()).Should(Equal(expectedBody), "Response body matches RootV3GetHandler response:")
		})

	})
}
