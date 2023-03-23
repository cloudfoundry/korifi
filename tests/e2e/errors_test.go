package e2e_test

import (
	"crypto/tls"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("404 errors", func() {
	var (
		client   *resty.Client
		httpResp *resty.Response
		method   string
		url      string
	)

	BeforeEach(func() {
		method = ""
		url = ""
		client = resty.New().
			SetBaseURL(apiServerRoot).
			SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	})

	JustBeforeEach(func() {
		var err error
		httpResp, err = client.R().Execute(method, url)
		Expect(err).NotTo(HaveOccurred())
	})

	When("an endpoint does not exist", func() {
		BeforeEach(func() {
			method = "GET"
			url = "/does-not-exist"
		})

		It("returns a valid CF 404 error", func() {
			Expect404(httpResp)
		})
	})

	When("a method is not supported", func() {
		BeforeEach(func() {
			method = "POST"
			url = "/v3"
		})

		It("returns a valid CF 404 error", func() {
			Expect404(httpResp)
		})
	})
})

func Expect404(httpResp *resty.Response) {
	Expect(httpResp).To(HaveRestyStatusCode(http.StatusNotFound))
	Expect(httpResp).To(HaveRestyBody(MatchJSON(`{
		"errors": [
			{
				"code": 10000,
				"detail": "Unknown request",
				"title": "CF-NotFound"
			}
		]
	}`)))
}
