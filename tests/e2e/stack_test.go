package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Stack", func() {
	var (
		httpResp    *resty.Response
		requestPath string
	)

	BeforeEach(func() {
		requestPath = "/v3/stacks"
	})
	JustBeforeEach(func() {
		var err error
		httpResp, err = adminClient.R().
			Get(requestPath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("succeeds", func() {
		Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
	})
})
