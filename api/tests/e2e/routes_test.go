package e2e_test

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Routes", func() {
	var (
		client     *resty.Client
		domainGUID string
		domainName string
		orgGUID    string
		spaceGUID  string
		host       string
		path       string
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)

		spaceGUID = createSpace(generateGUID("space"), orgGUID)

		domainName = generateGUID("domain-name")
		domainGUID = createDomain(domainName)

		host = generateGUID("myapp")
		path = generateGUID("/some-path")

		client = certClient
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
		deleteDomain(domainGUID)
	})

	Describe("fetch", func() {
		var (
			result    resource
			resp      *resty.Response
			errResp   cfErrs
			routeGUID string
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			routeGUID = createRoute(host, path, spaceGUID, domainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is authorized in the space", func() {
			It("can fetch the route", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(routeGUID))
			})
		})

		When("the user is not authorized in the space", func() {
			BeforeEach(func() {
				client = tokenClient
			})

			It("returns a not found error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
				Expect(errResp.Errors).To(ConsistOf(
					cfErr{
						Title:  "CF-ResourceNotFound",
						Code:   10010,
						Detail: "Route not found. Ensure it exists and you have access to it.",
					},
				))
			})
		})
	})

	Describe("create", func() {
		var (
			resp      *resty.Response
			createErr cfErrs
			route     routeResource
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetBody(routeResource{
					resource: resource{
						Relationships: map[string]relationship{
							"domain": {Data: resource{GUID: domainGUID}},
							"space":  {Data: resource{GUID: spaceGUID}},
						},
					},
					Host: host,
					Path: path,
				}).
				SetResult(&route).
				SetError(&createErr).
				Post("/v3/routes")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can create a route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(route.URL).To(SatisfyAll(
				HavePrefix(host),
				HaveSuffix(path),
			))
		})

		When("the route already exists", func() {
			BeforeEach(func() {
				createRoute(host, path, spaceGUID, domainGUID)
			})

			It("fails with a duplicate error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf("Route already exists with host '%s' and path '%s' for domain '%s'.", host, path, domainName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})

		When("the route already exists in another space", func() {
			BeforeEach(func() {
				anotherSpaceGUID := createSpace(generateGUID("another-space"), orgGUID)
				createRoute(host, path, anotherSpaceGUID, domainGUID)
			})

			It("fails with a duplicate error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf("Route already exists with host '%s' and path '%s' for domain '%s'.", host, path, domainName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})

		When("there is no context path", func() {
			BeforeEach(func() {
				path = ""
				createRoute(host, path, spaceGUID, domainGUID)
			})

			It("fails with a duplicate error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf("Route already exists with host '%s' for domain '%s'.", host, domainName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})
	})

	Describe("delete", func() {
		var (
			routeGUID string
			resp      *resty.Response
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			routeGUID = createRoute(host, path, spaceGUID, domainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				Delete("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can delete a route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
			Expect(resp).To(HaveRestyHeaderWithValue("Location", SatisfyAll(
				HavePrefix(apiServerRoot),
				ContainSubstring("/v3/jobs/route.delete-"),
			)))
		})

		It("frees up the deleted route's name for reuse", func() {
			createRoute(host, path, spaceGUID, domainGUID)
		})
	})

	Describe("add a destination", func() {
		var (
			appGUID   string
			routeGUID string
			resp      *resty.Response
			host      string
			result    destinationsResource
		)

		BeforeEach(func() {
			routeGUID = ""
			host = generateGUID("host")
			routeGUID = createRoute(host, "", spaceGUID, appDomainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(mapRouteResource{
					Destinations: []destinationRef{
						{App: resource{GUID: appGUID}},
					},
				}).
				SetResult(&result).
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

				appClient := resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
				Eventually(func() int {
					var err error
					resp, err = appClient.R().Get(fmt.Sprintf("https://%s.%s", host, appFQDN))
					if err != nil {
						return 0
					}
					return resp.StatusCode()
				}).Should(Equal(http.StatusOK))
				Expect(result.Destinations).To(HaveLen(1))
				Expect(result.Destinations[0].App.GUID).To(Equal(appGUID))

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
