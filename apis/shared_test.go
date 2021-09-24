package apis_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

func defaultServerURI(paths ...string) string {
	return fmt.Sprintf("%s%s", defaultServerURL, strings.Join(paths, ""))
}

func itRespondsWithUnknownError(it spec.S, g *WithT, rr func() *httptest.ResponseRecorder) {
	it("returns status 500 InternalServerError", func() {
		g.Expect(rr().Code).To(Equal(http.StatusInternalServerError), "Matching HTTP response code:")
	})

	it("returns Content-Type as JSON in header", func() {
		contentTypeHeader := rr().Header().Get("Content-Type")
		g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
	})

	it("returns a CF API formatted Error response", func() {
		g.Expect(rr().Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"title": "UnknownError",
						"detail": "An unknown error occurred.",
						"code": 10001
					}
				]
			}`), "Response body matches response:")
	})
}

func itRespondsWithNotFound(it spec.S, g *WithT, detail string, rr func() *httptest.ResponseRecorder) {
	it("returns status 404 NotFound", func() {
		g.Expect(rr().Code).To(Equal(http.StatusNotFound), "Matching HTTP response code:")
	})

	it("returns Content-Type as JSON in header", func() {
		contentTypeHeader := rr().Header().Get("Content-Type")
		g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
	})

	it("returns a CF API formatted Error response", func() {
		g.Expect(rr().Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"code": 10010,
						"title": "CF-ResourceNotFound",
						"detail": "`+detail+`"
					}
				]
			}`), "Response body matches response:")
	})
}
