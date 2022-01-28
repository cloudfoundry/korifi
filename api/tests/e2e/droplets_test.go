package e2e_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Droplets", func() {
	var (
		org   presenter.OrgResponse
		space presenter.SpaceResponse
	)

	BeforeEach(func() {
		org = createOrg(generateGUID("org"), adminAuthHeader)
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, adminAuthHeader)

		space = createSpace(generateGUID("space1"), org.GUID, adminAuthHeader)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space.GUID, adminAuthHeader)
	})

	AfterEach(func() {
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	Describe("get", func() {
		var (
			app   presenter.AppResponse
			pkg   presenter.PackageResponse
			build presenter.BuildResponse

			resp   map[string]interface{}
			getErr error
		)

		BeforeEach(func() {
			app = createApp(space.GUID, generateGUID("app"), adminAuthHeader)
			pkg = createPackage(app.GUID, adminAuthHeader)
			uploadNodeApp(pkg.GUID, adminAuthHeader)
			build = createBuild(pkg.GUID, adminAuthHeader)
		})

		JustBeforeEach(func() {
			Eventually(func() error {
				resp, getErr = get("/v3/droplets/"+build.GUID, adminAuthHeader)
				return getErr
			}).Should(Succeed())
		})

		It("returns the droplet", func() {
			Expect(resp).To(HaveKeyWithValue("guid", build.GUID))
			Expect(resp).To(HaveKeyWithValue("state", "STAGED"))
		})
	})
})
