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
		org      presenter.OrgResponse
		space    presenter.SpaceResponse
		app      presenter.AppResponse
		pkg      presenter.PackageResponse
		httpErr  error
		httpResp *resty.Response
		result   map[string]interface{}
	)

	BeforeEach(func() {
		org = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, adminAuthHeader)
		space = createSpace(generateGUID("space1"), org.GUID)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space.GUID, adminAuthHeader)
		app = createApp(space.GUID, generateGUID("app"))
		pkg = createPackage(app.GUID, adminAuthHeader)
	})

	AfterEach(func() {
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	Describe("get", func() {
		var build presenter.BuildResponse

		BeforeEach(func() {
			build = createBuild(pkg.GUID, adminAuthHeader)
		})

		JustBeforeEach(func() {
			httpResp, httpErr = certClient.R().
				SetResult(&result).
				Get("/v3/builds/" + build.GUID)
		})

		It("returns the build", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(result).To(HaveKeyWithValue("guid", build.GUID))
		})
	})

	Describe("create", func() {
		var (
			app presenter.AppResponse
			pkg presenter.PackageResponse
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
				SetResult(&result).
				Post("/v3/builds")
		})

		It("returns the build", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusCreated))
			Expect(result).To(HaveKeyWithValue("package", HaveKeyWithValue("guid", pkg.GUID)))
		})
	})
})
