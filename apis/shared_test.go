package apis_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

func defaultServerURI(paths ...string) string {
	return fmt.Sprintf("%s%s", defaultServerURL, strings.Join(paths, ""))
}

func itRespondsWithUnknownError(rr func() *httptest.ResponseRecorder) {
	It("returns status 500 InternalServerError", func() {
		Expect(rr().Code).To(Equal(http.StatusInternalServerError), "Matching HTTP response code:")
	})

	It("returns a CF API formatted Error response", func() {
		contentTypeHeader := rr().Header().Get("Content-Type")
		Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

		Expect(rr().Body.String()).To(MatchJSON(`{
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

func itRespondsWithNotFound(detail string, rr func() *httptest.ResponseRecorder) {
	It("returns status 404 NotFound", func() {
		Expect(rr().Code).To(Equal(http.StatusNotFound), "Matching HTTP response code:")
	})

	It("returns a CF API formatted Error response", func() {
		contentTypeHeader := rr().Header().Get("Content-Type")
		Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

		Expect(rr().Body.String()).To(MatchJSON(fmt.Sprintf(`{
			"errors": [
				{
					"code": 10010,
					"title": "CF-ResourceNotFound",
					"detail": %q
				}
			]
		}`, detail)), "Response body matches response:")
	})
}

func itRespondsWithUnprocessableEntity(detail string, rr func() *httptest.ResponseRecorder) {
	It("responds 422", func() {
		Expect(rr().Code).To(Equal(http.StatusUnprocessableEntity))
	})

	It("responds with a CF-UnprocessableEntity error", func() {
		contentTypeHeader := rr().Header().Get("Content-Type")
		Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

		Expect(rr().Body.String()).To(MatchJSON(fmt.Sprintf(`{
			"errors": [
				{
					"detail": %q,
					"title": "CF-UnprocessableEntity",
					"code": 10008
				}
			]
		}`, detail)))
	})
}
