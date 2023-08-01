package e2e_test

import (
	"crypto/tls"
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
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
		token := helpers.NewServiceAccountFactory(rootNamespace).CreateServiceAccount(uuid.NewString())
		client := helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).SetAuthToken(token).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

		var err error
		httpResp, err = client.R().Get(reqPath)
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
