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

func TestRootV3API(t *testing.T) {
	spec.Run(t, "the GET /v3 endpoint", testRootV3API, spec.Report(report.Terminal{}))
}

func testRootV3API(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var rr *httptest.ResponseRecorder
	it.Before(func() {
		req, err := http.NewRequest("GET", "/v3", nil)
		g.Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		router := mux.NewRouter()

		apiHandler := apis.NewRootV3Handler(defaultServerURL)
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

	it("matches the expected response body format", func() {
		expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `/v3"}}}`
		g.Expect(rr.Body.String()).To(Equal(expectedBody), "Response body matches RootV3GetHandler response:")
	})
}
