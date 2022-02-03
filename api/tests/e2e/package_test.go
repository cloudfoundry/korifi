package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Package", func() {
	var (
		orgGUID   string
		spaceGUID string
		appGUID   string
		resp      *resty.Response
		result    packageResource
		resultErr cfErrs
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)

		spaceGUID = createSpace(generateGUID("space1"), orgGUID)
		appGUID = createApp(spaceGUID, generateGUID("app"))
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	JustBeforeEach(func() {
		var err error
		resp, err = certClient.R().
			SetBody(packageResource{
				Type: "bits",
				resource: resource{
					Relationships: relationships{
						"app": relationship{Data: resource{GUID: appGUID}},
					},
				},
			}).
			SetError(&resultErr).
			SetResult(&result).
			Post("/v3/packages")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Create", func() {
		It("fails with a resource not found error", func() {
			Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
			Expect(resultErr.Errors).To(HaveLen(1))
			Expect(resultErr.Errors[0].Title).To(Equal("CF-ResourceNotFound"))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
				Expect(result.GUID).ToNot(BeEmpty())

				By("creating the package", func() {
					var actualPackage packageResource
					getResp, getErr := certClient.R().
						SetResult(&actualPackage).
						Get("/v3/packages/" + result.GUID)

					Expect(getErr).NotTo(HaveOccurred())
					Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
					Expect(actualPackage.GUID).To(Equal(result.GUID))
				})
			})
		})

		When("the user is a SpaceManager (i.e. can get apps but cannot create packages)", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("fails with a forbidden error", func() {
				Expect(resp.StatusCode()).To(Equal(http.StatusForbidden))
				Expect(resultErr.Errors).To(HaveLen(1))
				Expect(resultErr.Errors[0].Title).To(Equal("CF-NotAuthorized"))
			})
		})
	})
})
