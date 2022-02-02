package e2e_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Package", func() {
	var (
		org       presenter.OrgResponse
		space     presenter.SpaceResponse
		app       presenter.AppResponse
		httpResp  *resty.Response
		httpErr   error
		result    map[string]interface{}
		resultErr map[string]interface{}
	)

	BeforeEach(func() {
		org = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, org.GUID, adminAuthHeader)

		space = createSpace(generateGUID("space1"), org.GUID)
		app = createApp(space.GUID, generateGUID("app"))
	})

	AfterEach(func() {
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	JustBeforeEach(func() {
		httpResp, httpErr = certClient.R().
			SetBody(map[string]interface{}{
				"type": "bits",
				"relationships": map[string]interface{}{
					"app": map[string]interface{}{
						"data": map[string]interface{}{
							"guid": app.GUID,
						},
					},
				},
			}).
			SetError(&resultErr).
			SetResult(&result).
			Post("/v3/packages")
	})

	Describe("Create", func() {
		It("fails with a resource not found error", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusNotFound))
			Expect(resultErr).To(
				HaveKeyWithValue("errors", ConsistOf(
					HaveKeyWithValue("title", Equal("CF-ResourceNotFound")),
				)))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, space.GUID, adminAuthHeader)
			})

			It("succeeds", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusCreated))
				Expect(result).To(HaveKeyWithValue("guid", Not(BeEmpty())))

				By("creating the package", func() {
					actualPackage := map[string]interface{}{}
					getResp, getErr := certClient.R().SetResult(&actualPackage).Get(fmt.Sprintf("/v3/packages/%s", result["guid"]))
					Expect(getErr).NotTo(HaveOccurred())
					Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
					Expect(actualPackage["guid"]).To(Equal(result["guid"]))
				})
			})
		})

		When("the user is a SpaceManager (i.e. can get apps but cannot create packages)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, space.GUID, adminAuthHeader)
			})

			It("fails with a forbidden error", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusForbidden))
				Expect(resultErr).To(HaveKeyWithValue("errors", ConsistOf(HaveKeyWithValue("title", Equal("CF-NotAuthorized")))))
			})
		})
	})
})
