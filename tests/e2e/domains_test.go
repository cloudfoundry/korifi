package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Domain", func() {
	var (
		resp       *resty.Response
		domainName string
		domainGUID string
	)

	BeforeEach(func() {
		domainName = generateGUID("test-domain") + ".com"

		var err error
		var respResource responseResource
		resp, err = adminClient.R().
			SetBody(domainResource{
				resource: resource{Name: domainName},
				Internal: false,
			}).
			SetResult(&respResource).
			Post("/v3/domains")
		Expect(err).NotTo(HaveOccurred())

		Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

		domainGUID = respResource.GUID
	})

	AfterEach(func() {
		var err error
		resp, err = adminClient.R().
			Delete("/v3/domains/" + domainGUID)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(BeNumerically("<", http.StatusInternalServerError))
	})

	Describe("Create", func() {
		var (
			name         string
			guid         string
			respResource responseResource
			resultErr    cfErrs
		)

		BeforeEach(func() {
			name = generateGUID("create") + ".com"
			resultErr = cfErrs{}
		})

		AfterEach(func() {
			var err error
			resp, err = adminClient.R().
				Delete("/v3/domains/" + guid)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(domainResource{
					resource: resource{Name: name},
					Internal: false,
				}).
				SetResult(&respResource).
				SetError(&resultErr).
				Post("/v3/domains")
			Expect(err).NotTo(HaveOccurred())

			guid = respResource.GUID
		})

		It("creates the domain", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(respResource.GUID).To(Equal(guid))
			Expect(respResource.Name).To(Equal(name))
		})
	})

	Describe("Get", func() {
		var respResource responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&respResource).
				Get("/v3/domains/" + domainGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 200 OK", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(respResource.GUID).To(Equal(domainGUID))
		})
	})

	Describe("Update", func() {
		var respResource responseResource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(metadataResource{
					Metadata: &metadataPatch{
						Annotations: &map[string]string{"foo": "bar"},
						Labels:      &map[string]string{"baz": "bar"},
					},
				}).
				SetResult(&respResource).
				Patch("/v3/domains/" + domainGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 200 OK and updates domain labels and annotations", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(respResource.GUID).NotTo(BeEmpty())
			Expect(respResource.Name).To(Equal(domainName))
			Expect(respResource.Metadata.Annotations).To(HaveKeyWithValue("foo", "bar"))
			Expect(respResource.Metadata.Labels).To(HaveKeyWithValue("baz", "bar"))
		})
	})

	Describe("List", func() {
		var result resourceList[responseResource]

		BeforeEach(func() {
			result = resourceList[responseResource]{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/domains")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a list of domains that includes the created domains", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(appFQDN)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(domainName)}),
			))
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				Delete("/v3/domains/" + domainGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/domain.delete~"+domainGUID)),
			))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				var err error
				resp, err = adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(resp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			getDomainResp, err := adminClient.R().Get("/v3/domains/" + domainGUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(getDomainResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})

		When("the domain has routes", func() {
			var spaceGUID string
			BeforeEach(func() {
				spaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)

				var createRouteErr cfErrs
				createRouteResp, err := adminClient.R().
					SetBody(routeResource{
						resource: resource{
							Relationships: map[string]relationship{
								"domain": {Data: resource{GUID: domainGUID}},
								"space":  {Data: resource{GUID: spaceGUID}},
							},
						},
						Host: "my-host",
						Path: "/foo",
					}).
					SetError(&createRouteErr).
					Post("/v3/routes")
				Expect(err).NotTo(HaveOccurred())
				Expect(createRouteErr.Errors).To(BeEmpty())
				Expect(createRouteResp).To(HaveRestyStatusCode(http.StatusCreated))
			})

			It("deletes the domain routes", func() {
				Eventually(func(g Gomega) {
					var routes resourceList[responseResource]
					listRoutesResp, err := adminClient.R().
						SetResult(&routes).
						Get("/v3/routes?space_guids=" + spaceGUID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(listRoutesResp).To(HaveRestyStatusCode(http.StatusOK))
					g.Expect(routes.Resources).To(BeEmpty())
				}).Should(Succeed())
			})
		})
	})
})
