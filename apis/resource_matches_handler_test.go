package apis_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		rr         *httptest.ResponseRecorder
		apiHandler *ResourceMatchesHandler
	)

	makePostRequest := func(body string) {
		req, err := http.NewRequest("POST", "unused-path", strings.NewReader(body))
		g.Expect(err).NotTo(HaveOccurred())

		handler := http.HandlerFunc(apiHandler.ResourceMatchesPostHandler)
		handler.ServeHTTP(rr, req)
	}

	it.Before(func() {
		rr = httptest.NewRecorder()
		apiHandler = &ResourceMatchesHandler{}
	})

	when("ResourceMatchesHandler is called", func() {
		it.Before(func() {
			makePostRequest(`{}`)
		})

		it("returns status 201 Created", func() {
			g.Expect(rr.Code).Should(Equal(http.StatusCreated), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).Should(MatchJSON(`{
				"resources": []
			  }`), "Response body matches response:")
		})
	})
}
