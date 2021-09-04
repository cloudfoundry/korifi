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

func TestRootAPI(t *testing.T) {
	spec.Run(t, "object", testRootV3API, spec.Report(report.Terminal{}))
}

func testRootAPI(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("the root GET endpoint returns successfully", func() {
		var rr *httptest.ResponseRecorder

		it.Before(func() {
			// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
			// pass 'nil' as the third parameter.
			req, err := http.NewRequest("GET", "/", nil)
			g.Expect(err).NotTo(HaveOccurred())

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
			g.Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has a non-empty body", func() {
			responseBody := rr.Body.Bytes()
			g.Expect(responseBody).NotTo(BeEmpty())
		})

		it("matches the expected response body format", func() {
			expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `"},"bits_service":null,"cloud_controller_v2":null,"cloud_controller_v3":{"href":"` + defaultServerURL + `/v3","meta":{"version":"3.90.0"}},"network_policy_v0":null,"network_policy_v1":null,"login":null,"uaa":null,"credhub":null,"routing":null,"logging":null,"log_cache":null,"log_stream":null,"app_ssh":null}}`
			g.Expect(rr.Body.String()).Should(Equal(expectedBody), "Response body matches RootV3GetHandler response:")
		})
	})
}
