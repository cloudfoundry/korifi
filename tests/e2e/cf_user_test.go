package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CF User", func() {
	var (
		httpResp *resty.Response
		reqPath  string
	)

	BeforeEach(func() {
		reqPath = "/v3/apps"
	})

	JustBeforeEach(func() {
		var err error
		httpResp, err = unprivilegedServiceAccountClient.R().Get(reqPath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("sets a X-Cf-Warnings header", func() {
		Expect(httpResp).To(HaveRestyHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
	})

	When("an unauthenticated endpoint is requested ", func() {
		BeforeEach(func() {
			reqPath = "/"
		})

		It("succeeds", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
		})

		It("does not set a X-Cf-Warnings", func() {
			Expect(httpResp.Header()).NotTo(HaveKey("X-Cf-Warnings"))
		})
	})
})
