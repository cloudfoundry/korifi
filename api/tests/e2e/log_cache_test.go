package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
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
		appGUID = pushTestApp(spaceGUID)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Get", func() {
		var result appLogResource
		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().SetResult(&result).Get("/api/v1/read/" + appGUID)
		})

		It("succeeds with log envelopes", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Envelopes.Batch).NotTo(BeEmpty())
		})
	})
})
