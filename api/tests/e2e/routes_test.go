package e2e_test

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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

		domainName = generateGUID("domain.name")
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

	Describe("list", func() {
		var (
			result  responseResourceList
			resp    *resty.Response
			errResp cfErrs

			route1AGUID, route1BGUID string

			space2GUID  string
			route2AGUID string
		)

		BeforeEach(func() {
			host1 := generateGUID("myapp1")
			route1AGUID = createRoute(host1, generateGUID("/some-path"), spaceGUID, domainGUID)
			route1BGUID = createRoute(host1, generateGUID("/some-path"), spaceGUID, domainGUID)

			space2GUID = createSpace(generateGUID("space"), orgGUID)
			host2 := generateGUID("myapp2")
			route2AGUID = createRoute(host2, generateGUID("/some-path"), space2GUID, domainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is authorized within one space, but not another", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("returns the list of routes in only the authorized spaces", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(route1AGUID)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(route1BGUID)}),
				))
				Expect(result.Resources).ToNot(ContainElement(MatchFields(IgnoreExtras, Fields{"GUID": Equal(route2AGUID)})))
			})
		})

		When("the user is not authorized in any space", func() {
			BeforeEach(func() {
				client = tokenClient
			})

			It("returns an empty list", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(BeEmpty())
			})
		})
	})

	Describe("create", func() {
		var (
			resp      *resty.Response
			createErr cfErrs
			route     routeResource
		)

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

		It("returns an unprocessable entity error when the user has no role in the space", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
			Expect(createErr.Errors).To(ConsistOf(cfErr{
				Detail: "Invalid space. Ensure that the space exists and you have access to it.",
				Title:  "CF-UnprocessableEntity",
				Code:   10008,
			}))
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("returns an unprocessable entity error when the user has no role in the space", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
				Expect(resp).To(HaveRestyBody(ContainSubstring("CF-NotAuthorized")))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("can create a route", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(route.URL).To(SatisfyAll(
					HavePrefix(host),
					HaveSuffix(path),
				))
				Expect(route.GUID).To(HavePrefix("cf-route-"))
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

			When("the FQDN on the route is invalid", func() {
				BeforeEach(func() {
					domainName = generateGUID("inv@lid.dom@in.n@me")
					domainGUID = createDomain(domainName)
				})

				It("fails with a invalid route error", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
					Expect(createErr.Errors).To(ConsistOf(cfErr{
						Detail: "Invalid Route, Route FQDN does not comply with RFC 1035 standards",
						Title:  "CF-UnprocessableEntity",
						Code:   10008,
					}))
				})
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
			errResp   cfErrs
			result    destinationsResource
		)

		BeforeEach(func() {
			routeGUID = ""
			host = generateGUID("host")
			routeGUID = createRoute(host, "", spaceGUID, appDomainGUID)
			errResp = cfErrs{}
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
				SetError(&errResp).
				Post("/v3/routes/" + routeGUID + "/destinations")

			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a space developer in the space", func() {
			BeforeEach(func() {
				appGUID = pushTestApp(spaceGUID)
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("returns success and routes the host to the app", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

				appClient := resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
				Eventually(func(g Gomega) {
					var err error
					resp, err = appClient.R().Get(fmt.Sprintf("https://%s.%s", host, appFQDN))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				}).Should(Succeed())
				Expect(result.Destinations).To(HaveLen(1))
				Expect(result.Destinations[0].App.GUID).To(Equal(appGUID))

				Expect(resp.Body()).To(ContainSubstring("hello-world"))
			})

			When("an app from a different space is added", func() {
				BeforeEach(func() {
					space2GUID := createSpace(generateGUID("space2"), orgGUID)
					appGUID = createApp(space2GUID, generateGUID("app2"))
				})

				It("fails with an unprocessable entity error", func() {
					expectUnprocessableEntityError(resp, errResp, "Routes cannot be mapped to destinations in different spaces.")
				})
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

	Describe("list destinations", func() {
		var (
			appGUID          string
			routeGUID        string
			destinationGUIDs []string
			errResp          cfErrs
			result           destinationsResource
			resp             *resty.Response
		)

		BeforeEach(func() {
			appGUID = createApp(spaceGUID, generateGUID("app"))
			routeGUID = createRoute(host, generateGUID("/some-path"), spaceGUID, domainGUID)
			destinationGUIDs = addDestinationForRoute(appGUID, routeGUID)
			Expect(destinationGUIDs).To(HaveLen(1))
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes/" + routeGUID + "/destinations")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is a space developer in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("returns the destinations", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Destinations).To(ConsistOf(MatchFields(IgnoreExtras, Fields{"GUID": Equal(destinationGUIDs[0])})))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns resource not found response", func() {
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
})
