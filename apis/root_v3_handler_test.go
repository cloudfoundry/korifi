package apis_test

import (
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RootV3Handler", func() {
	Describe("the GET /v3 endpoint", func() {
		var rr *httptest.ResponseRecorder
		BeforeEach(func() {
			req, err := http.NewRequest("GET", "/v3", nil)
			Expect(err).NotTo(HaveOccurred())

			rr = httptest.NewRecorder()
			router := mux.NewRouter()

			apiHandler := apis.NewRootV3Handler(defaultServerURL)
			apiHandler.RegisterRoutes(router)

			router.ServeHTTP(rr, req)
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		It("matches the expected response body format", func() {
			expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `/v3"}}}`
			Expect(rr.Body.String()).To(Equal(expectedBody), "Response body matches RootV3GetHandler response:")
		})
	})
})
