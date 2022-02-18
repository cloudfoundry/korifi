package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Routes", func() {
	var (
		orgGUID    string
		spaceGUID  string
		domainGUID string
		routeGUID  string
		resp       *resty.Response
		errResp    cfErrs
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)

		spaceGUID = createSpace(generateGUID("space"), orgGUID)

		domainGUID = createDomain("example.org")
		routeGUID = createRoute("my-app", "", spaceGUID, domainGUID)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("Fetch an route", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is authorized in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("can fetch the route", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(routeGUID))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a not found error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
				Expect(errResp.Errors).To(ConsistOf(
					cfErr{
						Detail: "Route not found",
						Title:  "CF-ResourceNotFound",
						Code:   10010,
					},
				))
			})
		})
	})
})
