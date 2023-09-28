package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Authorization", func() {
	var (
		userName   string
		userClient *helpers.CorrelatedRestyClient

		reqErr error
		resp   *resty.Response
	)

	BeforeEach(func() {
		userName = uuid.NewString()
		userClient = makeTokenClient(serviceAccountFactory.CreateServiceAccount(userName))
	})

	AfterEach(func() {
		serviceAccountFactory.DeleteServiceAccount(userName)
	})

	Describe("Unauthorized users", func() {
		When("an authenticated endpoint is requested", func() {
			BeforeEach(func() {
				resp, reqErr = userClient.R().Get("/v3/apps")
				Expect(reqErr).NotTo(HaveOccurred())
			})

			It("sets a X-Cf-Warnings header", func() {
				Expect(resp).To(HaveRestyHeaderWithValue("X-Cf-Warnings", ContainSubstring("has no CF roles assigned")))
			})
		})

		When("an unauthenticated endpoint is requested ", func() {
			BeforeEach(func() {
				resp, reqErr = userClient.R().Get("/")
				Expect(reqErr).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			})

			It("does not set a X-Cf-Warnings", func() {
				Expect(resp.Header()).NotTo(HaveKey("X-Cf-Warnings"))
			})
		})
	})

	Describe("Space Developer", func() {
		var (
			orgGUID   string
			spaceGUID string

			resp *resty.Response
		)

		BeforeEach(func() {
			orgName := generateGUID("org")
			orgGUID = createOrg(orgName)
			createOrgRole("organization_user", serviceAccountFactory.FullyQualifiedName(userName), orgGUID)

			spaceName := generateGUID("space")
			spaceGUID = createSpace(spaceName, orgGUID)
			createSpaceRole("space_developer", serviceAccountFactory.FullyQualifiedName(userName), spaceGUID)
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		It("can create an app", func() {
			resp, reqErr = userClient.R().SetBody(appResource{
				resource: resource{
					Name: "test-app",
					Relationships: relationships{
						"space": {
							Data: resource{
								GUID: spaceGUID,
							},
						},
					},
				},
			}).Post("/v3/apps")
			Expect(reqErr).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
		})

		It("cannot create an org", func() {
			resp, reqErr = userClient.R().SetBody(resource{
				Name: "test-org",
			}).Post("/v3/organizations")
			Expect(reqErr).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
		})
	})
})
