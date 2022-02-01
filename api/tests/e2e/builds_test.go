package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Builds", func() {
	var (
		org   presenter.OrgResponse
		space presenter.SpaceResponse
	)

	BeforeEach(func() {
		org = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, adminAuthHeader)

		space = createSpace(generateGUID("space1"), org.GUID)
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
			app = createApp(space.GUID, generateGUID("app"))
			pkg = createPackage(app.GUID, adminAuthHeader)
			build = createBuild(pkg.GUID, adminAuthHeader)
		})

		JustBeforeEach(func() {
			resp, getErr = get("/v3/builds/"+build.GUID, certAuthHeader)
		})

		It("returns the build", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(resp).To(HaveKeyWithValue("guid", build.GUID))
		})
	})

	Describe("create", func() {
		var (
			app      presenter.AppResponse
			pkg      presenter.PackageResponse
			httpResp *resty.Response
			httpErr  error

			resp map[string]interface{}
		)

		BeforeEach(func() {
			app = createApp(space.GUID, generateGUID("app"))
			pkg = createPackage(app.GUID, adminAuthHeader)
		})

		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().
				SetBody(map[string]interface{}{
					"package": map[string]interface{}{
						"guid": pkg.GUID,
					},
				}).
				SetResult(&resp).
				Post("/v3/builds")
		})

		It("returns the build", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusCreated))
			Expect(resp).To(HaveKeyWithValue("package", HaveKeyWithValue("guid", pkg.GUID)))
		})
	})
})
