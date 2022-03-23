package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Droplets", func() {
	var (
		orgGUID   string
		spaceGUID string
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)

		spaceGUID = createSpace(generateGUID("space1"), orgGUID)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("get", func() {
		var (
			buildGUID string
			result    resource
		)

		BeforeEach(func() {
			appGUID := createApp(spaceGUID, generateGUID("app"))
			pkgGUID := createPackage(appGUID)
			uploadNodeApp(pkgGUID)
			buildGUID = createBuild(pkgGUID)
		})

		JustBeforeEach(func() {
			Eventually(func() (*resty.Response, error) {
				return certClient.R().
					SetResult(&result).
					Get("/v3/droplets/" + buildGUID)
			}).Should(HaveRestyStatusCode(http.StatusOK))
		})

		It("returns the droplet", func() {
			Expect(result.GUID).To(Equal(buildGUID))
		})
	})
})
