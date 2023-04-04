package e2e_test

import (
	"crypto/tls"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Min CF CLI Version", func() {
	var (
		client   *resty.Client
		httpResp *resty.Response
	)

	BeforeEach(func() {
		client = resty.New().SetBaseURL(apiServerRoot).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	})

	JustBeforeEach(func() {
		var err error
		httpResp, err = client.SetHeader("User-Agent", "cf/0.0.1").R().Get("/v3")
		Expect(err).NotTo(HaveOccurred())
	})

	It("validates the minimum cli version", func() {
		Expect(httpResp).To(HaveRestyStatusCode(http.StatusBadRequest))
	})
})
