package e2e_test

import (
	"crypto/tls"
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Routes", func() {
	var (
		client     *helpers.CorrelatedRestyClient
		domainGUID string
		domainName string
		spaceGUID  string
		host       string
		path       string
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)

		domainName = helpers.GetRequiredEnvVar("APP_FQDN")
		domainGUID = getDomainGUID(domainName)

		host = generateGUID("myapp")
		path = generateGUID("/some-path")

		client = certClient
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("fetch", func() {
		var (
			result    resource
			resp      *resty.Response
			errResp   cfErrs
			routeGUID string
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
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

		It("can fetch the route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(routeGUID))
		})
	})

	Describe("list", func() {
		var (
			result  resourceList[responseResource]
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

			space2GUID = createSpace(generateGUID("space"), commonTestOrgGUID)
			host2 := generateGUID("myapp2")
			route2AGUID = createRoute(host2, generateGUID("/some-path"), space2GUID, domainGUID)

			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

		AfterEach(func() {
			deleteSpace(space2GUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the list of routes", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(route1AGUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(route1BGUID)}),
			))
			Expect(result.Resources).ToNot(ContainElement(MatchFields(IgnoreExtras, Fields{"GUID": Equal(route2AGUID)})))
		})
	})

	Describe("create", func() {
		var (
			resp      *resty.Response
			createErr cfErrs
			route     routeResource
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
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

		It("creates a route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(route.URL).To(SatisfyAll(
				HavePrefix(host),
				HaveSuffix(path),
			))
			Expect(route.GUID).To(HavePrefix("cf-route-"))
		})
	})

	Describe("delete", func() {
		var (
			routeGUID string
			resp      *resty.Response
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
			routeGUID = createRoute(host, path, spaceGUID, domainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				Delete("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes the route and redirects to a deletion job", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
			Expect(resp).To(HaveRestyHeaderWithValue("Location", SatisfyAll(
				HavePrefix(apiServerRoot),
				ContainSubstring("/v3/jobs/route.delete~"+routeGUID),
			)))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				jobResp, err := client.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			getRouteResp, err := client.R().Get("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(getRouteResp).To(HaveRestyStatusCode(http.StatusNotFound))

			By("freeing up the deleted route's name for reuse", func() {
				createRoute(host, path, spaceGUID, domainGUID)
			})
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
			routeGUID = createRoute(host, "", spaceGUID, domainGUID)
			errResp = cfErrs{}

			appGUID, _ = pushTestApp(spaceGUID, defaultAppBitsFile)
			createSpaceRole("space_developer", certUserName, spaceGUID)
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

		It("returns success and routes the host to the app", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

			appClient := resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
			Eventually(func(g Gomega) {
				var err error
				resp, err = appClient.R().
					SetPathParam("host", host).
					SetPathParam("appFQDN", appFQDN).
					Get("https://{host}.{appFQDN}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			}).Should(Succeed())
			Expect(result.Destinations).To(HaveLen(1))
			Expect(result.Destinations[0].App.GUID).To(Equal(appGUID))

			// This enables replacing the default app output via DEFAULT_APP_BITS_PATH and DEFAULT_APP_RESPONSE
			Expect(resp.Body()).To(ContainSubstring(helpers.GetDefaultedEnvVar("DEFAULT_APP_RESPONSE", "Hi, I'm Dorifi")))
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

			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes/" + routeGUID + "/destinations")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the destinations", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Destinations).To(ConsistOf(MatchFields(IgnoreExtras, Fields{"GUID": Equal(destinationGUIDs[0])})))
		})
	})

	Describe("delete destination", func() {
		var (
			appGUID          string
			routeGUID        string
			destinationGUIDs []string
			errResp          cfErrs
			resp             *resty.Response
		)

		BeforeEach(func() {
			appGUID = createApp(spaceGUID, generateGUID("app"))
			routeGUID = createRoute(host, generateGUID("/some-path"), spaceGUID, domainGUID)
			destinationGUIDs = addDestinationForRoute(appGUID, routeGUID)
			Expect(destinationGUIDs).To(HaveLen(1))

			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetError(&errResp).
				Delete("/v3/routes/" + routeGUID + "/destinations/" + destinationGUIDs[0])
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with 204 No Content", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusNoContent))
		})
	})
})
