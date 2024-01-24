package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type RootV3Resource struct {
	Links struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}

var _ = Describe("RootV3", func() {
	var (
		httpResp    *resty.Response
		requestPath string
	)

	BeforeEach(func() {
		requestPath = "/v3"
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

	When("Request ends with a trailing slash", func() {
		BeforeEach(func() {
			requestPath = "/v3/"
		})
		It("succeeds", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
		})
	})
})
