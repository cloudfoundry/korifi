package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Domain", func() {
	var (
		restyClient *resty.Client
	)

	BeforeEach(func() {
		restyClient = certClient
	})

	Describe("list", func() {
		var (
			result responseResourceList
			resp   *resty.Response

			domain1GUID string
			domain2GUID string
		)

		BeforeEach(func() {
			domain1Name := generateGUID("domain-1-name")
			domain1GUID = createDomain(domain1Name)
			domain2Name := generateGUID("domain-2-name")
			domain2GUID = createDomain(domain2Name)
		})

		AfterEach(func() {
			deleteDomain(domain1GUID)
			deleteDomain(domain2GUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetResult(&result).
				Get("/v3/domains")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has acquired the cf_user role", func() {
			var (
				orgGUID string
			)

			BeforeEach(func() {
				orgGUID = createOrg(generateGUID("org"))
				createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
			})

			AfterEach(func() {
				deleteOrg(orgGUID)
			})

			It("returns a list of domains that includes the created domains", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(domain1GUID)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(domain2GUID)}),
				))
			})
		})

		When("the user has no permissions", func() {
			BeforeEach(func() {
				restyClient = tokenClient
			})

			It("returns an empty list", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(BeEmpty())
			})
		})
	})
})
