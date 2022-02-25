package e2e_test

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Routes", func() {
	var (
		orgGUID   string
		spaceGUID string
		appGUID   string
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
		spaceGUID = createSpace(generateGUID("space"), orgGUID)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("adding a destination", func() {
		var (
			routeGUID string
			resp      *resty.Response
			host      string
		)

		BeforeEach(func() {
			routeGUID = ""
			host = generateGUID("host")
			routeGUID = createRoute(host, spaceGUID)
		})

		JustBeforeEach(func() {
			var route resource
			var err error
			resp, err = certClient.R().
				SetBody(mapRouteResource{
					Destinations: []destinationRef{
						{App: resource{GUID: appGUID}},
					},
				}).
				SetResult(&route).
				Post("/v3/routes/" + routeGUID + "/destinations")

			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a space developer in the space", func() {
			BeforeEach(func() {
				appGUID = pushNodeApp(spaceGUID)
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("returns success and routes the host to the app", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

				appClient := resty.New()
				Eventually(func() int {
					var err error
					resp, err = appClient.R().Get(fmt.Sprintf("http://%s.%s", host, SamplesDomain))
					Expect(err).NotTo(HaveOccurred())
					return resp.StatusCode()
				}).Should(Equal(http.StatusOK))

				Expect(resp.Body()).To(ContainSubstring("Hello from a node app!"))
			})
		})

		When("the user is a space manager in the space", func() {
			BeforeEach(func() {
				appGUID = createApp(spaceGUID, generateGUID("app"))
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("returns a forbidden response", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})

		When("the user has no access to the space", func() {
			BeforeEach(func() {
				appGUID = createApp(spaceGUID, generateGUID("app"))
			})

			It("returns a not found response", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
			})
		})
	})
})
