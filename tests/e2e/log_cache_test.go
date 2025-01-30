package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCache", func() {
	var (
		appGUID   string
		spaceGUID string
		httpResp  *resty.Response
		httpError error
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID, _ = pushTestApp(spaceGUID, defaultAppBitsFile)
	})

	Describe("Get", func() {
		var result logCacheResponse

		It("succeeds with log envelopes that include both app and staging logs", func() {
			httpResp, httpError = adminClient.R().SetResult(&result).Get("/api/v1/read/" + appGUID)
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Envelopes.Batch).NotTo(BeEmpty())
		})
	})
})
