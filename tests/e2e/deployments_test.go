package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Deployments", func() {
	var (
		spaceGUID string
		appGUID   string
		resp      *resty.Response
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("deployments-space"), commonTestOrgGUID)
		appGUID, _ = pushTestApp(spaceGUID, defaultAppBitsFile)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Get", func() {
		var (
			deploymentGUID string
			deploymentResp responseResource
		)

		BeforeEach(func() {
			deploymentGUID = createDeployment(appGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&deploymentResp).
				Get("/v3/deployments/" + deploymentGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 200 OK", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(deploymentResp.GUID).To(Equal(deploymentGUID))
		})
	})

	Describe("Create", func() {
		var (
			deploymentResource resource
			createResp         *resty.Response
		)

		BeforeEach(func() {
			var err error
			createResp, err = adminClient.R().
				SetBody(resource{
					Relationships: relationships{
						"app": relationship{
							Data: resource{
								GUID: appGUID,
							},
						},
					},
				}).
				SetResult(&deploymentResource).
				Post("/v3/deployments")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 201 Created", func() {
			Expect(createResp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(deploymentResource.GUID).NotTo(BeEmpty())
		})
	})
})
