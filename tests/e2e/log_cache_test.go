package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
		appGUID, _ = pushTestApp(spaceGUID, nodeAppBitsFile)
		createSpaceRole("space_developer", certUserName, spaceGUID)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Get", func() {
		var result appLogResource

		It("succeeds with log envelopes that include both app and staging logs", func() {
			Eventually(func(g Gomega) {
				httpResp, httpError = certClient.R().SetResult(&result).Get("/api/v1/read/" + appGUID)
				g.Expect(httpError).NotTo(HaveOccurred())
				g.Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
				g.Expect(result.Envelopes.Batch).NotTo(BeEmpty())
				g.Expect(result.Envelopes.Batch).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"Tags": HaveKeyWithValue("source_type", "STG"),
					})))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				httpResp, httpError = certClient.R().SetResult(&result).Get("/api/v1/read/" + appGUID)
				g.Expect(httpError).NotTo(HaveOccurred())
				g.Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
				g.Expect(result.Envelopes.Batch).NotTo(BeEmpty())
				g.Expect(result.Envelopes.Batch).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"Tags": HaveKeyWithValue("source_type", "APP"),
					})))
			}).Should(Succeed())
		})
	})
})
