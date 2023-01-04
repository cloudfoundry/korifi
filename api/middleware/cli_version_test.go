package middleware_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/middleware"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CliVersionMiddleware", func() {
	var (
		nextHandler    http.Handler
		requestHeaders map[string][]string
	)

	BeforeEach(func() {
		requestHeaders = map[string][]string{}
		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	JustBeforeEach(func() {
		request, err := http.NewRequest(http.MethodGet, "http://localhost/foo", nil)
		request.Header = requestHeaders
		Expect(err).NotTo(HaveOccurred())
		middleware.CFCliVersion(nextHandler).ServeHTTP(rr, request)
	})

	It("delegates to the next handler", func() {
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	When("the user agent is not formatted properly", func() {
		BeforeEach(func() {
			requestHeaders[headers.UserAgent] = []string{"i-am-not-formatted-properly"}
		})

		It("delegates to the next handler", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})

	When("the user agent is not cf cli", func() {
		BeforeEach(func() {
			requestHeaders[headers.UserAgent] = []string{"curl/7.81.0"}
		})

		It("delegates to the next handler", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})
	})

	When("the user agent is cf cli", func() {
		BeforeEach(func() {
			requestHeaders[headers.UserAgent] = []string{"cf/8.5.0+73aa161.2022-09-12 (go1.18.5; amd64 linux)"}
		})

		It("delegates to the next handler", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
		})

		When("the cf cli version is too old", func() {
			BeforeEach(func() {
				requestHeaders[headers.UserAgent] = []string{"cf/8.4.0+73aa161.2022-09-12 (go1.18.5; amd64 linux)"}
			})

			It("returns an informative error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
				Expect(rr).To(HaveHTTPBody(ContainSubstring("Korifi requires CF CLI >= 8.5.0")))
			})
		})

		When("there are  multiple UserAgent header values", func() {
			BeforeEach(func() {
				requestHeaders[headers.UserAgent] = []string{
					"cf/8.5.0+73aa161.2022-09-12 (go1.18.5; amd64 linux)",
					"cf/8.4.0+73aa161.2022-09-12 (go1.18.5; amd64 linux)",
				}
			})

			It("respects the first one", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
			})
		})

		When("the cf cli version cannot be parsed", func() {
			BeforeEach(func() {
				requestHeaders[headers.UserAgent] = []string{"cf/unknown"}
			})

			It("returns an informative error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
				Expect(rr).To(HaveHTTPBody(ContainSubstring("Korifi requires CF CLI >= 8.5.0")))
			})
		})
	})
})
