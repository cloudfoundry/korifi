package apis_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	. "code.cloudfoundry.org/cf-k8s-api/apis"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestResourceMatches(t *testing.T) {
	spec.Run(t, "ResourceMatchesHandler", testResourceMatchesHandler, spec.Report(report.Terminal{}))
}

func testResourceMatchesHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var (
		rr     *httptest.ResponseRecorder
		router *mux.Router
	)

	makePostRequest := func(body string) {
		req, err := http.NewRequest("POST", "/v3/resource_matches", strings.NewReader(body))
		g.Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	}

	it.Before(func() {
		rr = httptest.NewRecorder()
		router = mux.NewRouter()
		apiHandler := &ResourceMatchesHandler{}
		apiHandler.RegisterRoutes(router)
	})

	when("ResourceMatchesHandler is called", func() {
		it.Before(func() {
			makePostRequest(`{}`)
		})

		it("returns status 201 Created", func() {
			g.Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				"resources": []
			  }`), "Response body matches response:")
		})
	})
}
