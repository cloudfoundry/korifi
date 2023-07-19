package e2e_test

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Bindings", func() {
	var (
		appGUID      string
		spaceGUID    string
		bindingGUID  string
		instanceGUID string
		httpResp     *resty.Response
		httpError    error
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
		appGUID = createApp(spaceGUID, generateGUID("app"))
		instanceGUID = createServiceInstance(spaceGUID, generateGUID("service-instance"), nil)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("POST /v3/service_credential_bindings/{guid}", func() {
		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().
				SetBody(typedResource{
					Type: "app",
					resource: resource{
						Relationships: relationships{"app": {Data: resource{GUID: appGUID}}, "service_instance": {Data: resource{GUID: instanceGUID}}},
					},
				}).
				Post("/v3/service_credential_bindings")
		})

		It("returns a not found error when the user has no role in the space", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusForbidden))
		})

		When("the user has space manager role", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("returns a forbidden error", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})

		When("the user has space developer role", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusCreated))
			})

			When("the user attempts to create a duplicate service binding", func() {
				BeforeEach(func() {
					_ = createServiceBinding(appGUID, instanceGUID, "")
				})

				It("returns an error", func() {
					Expect(httpError).NotTo(HaveOccurred())
					Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				})
			})
		})
	})

	Describe("GET /v3/service_credential_bindings/{guid}", func() {
		var respResource responseResource

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")
		})

		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().
				SetResult(&respResource).
				Get("/v3/service_credential_bindings/" + bindingGUID)
		})

		It("gets the service binding", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(respResource.GUID).To(Equal(bindingGUID))
		})
	})

	Describe("DELETE /v3/service_credential_bindings/{guid}", func() {
		BeforeEach(func() {
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")
		})

		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().Delete("/v3/service_credential_bindings/" + bindingGUID)
		})

		It("returns a not found error when the user has no role in the space", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})

		When("the user has space manager role", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("returns a forbidden error", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})

		When("the user has space developer role", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusNoContent))
			})
		})
	})

	Describe("GET /v3/service_credential_bindings", func() {
		var (
			anotherInstanceGUID string
			anotherBindingGUID  string
			queryString         string
			result              resourceListWithInclusion
		)

		BeforeEach(func() {
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")

			anotherInstanceGUID = createServiceInstance(spaceGUID, generateGUID("another-service-instance"), nil)
			anotherBindingGUID = createServiceBinding(appGUID, anotherInstanceGUID, "")

			queryString = ""
			result = resourceListWithInclusion{}
		})

		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().SetResult(&result).Get("/v3/service_credential_bindings" + queryString)
		})

		It("returns a list without ServiceBindings in spaces where the user doesn't have access", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).NotTo(ContainElement(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(bindingGUID)}),
			))
		})

		When("the user has space manager role", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(bindingGUID)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(anotherBindingGUID)}),
				))
			})
		})

		When("the user has space developer role", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(bindingGUID)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(anotherBindingGUID)}),
				))
			})

			It("doesn't return anything in the 'included' list", func() {
				Expect(result.Included).To(BeNil())
			})

			When("the 'include=app' querystring is set", func() {
				BeforeEach(func() {
					queryString = `?include=app`
				})

				It("returns an app in the 'included' list", func() {
					Expect(httpError).NotTo(HaveOccurred())
					Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
					Expect(result.Resources).NotTo(BeEmpty())
					Expect(result.Included).NotTo(BeNil())
					Expect(result.Included.Apps).To(ContainElement(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(appGUID)}),
					))
				})
			})

			When("label selector is specified on the search query", func() {
				var label string

				BeforeEach(func() {
					label = uuid.NewString()
					queryString = fmt.Sprintf(`?label_selector=%s`, label)
					addServiceBindingLabels(bindingGUID, map[string]string{label: "whatever"})
				})

				It("only returns the bindings that have that label", func() {
					Expect(result.Resources).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(bindingGUID)}),
					))
				})
			})
		})
	})

	Describe("PATCH /v3/service_credential_bindings/{guid}", func() {
		var respResource responseResource

		BeforeEach(func() {
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")
			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			httpResp, err = certClient.R().
				SetBody(metadataResource{
					Metadata: &metadataPatch{
						Annotations: &map[string]string{"foo": "bar"},
						Labels:      &map[string]string{"baz": "bar"},
					},
				}).
				SetResult(&respResource).
				Patch("/v3/service_credential_bindings/" + bindingGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 200 OK and updates service binding labels and annotations", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(respResource.GUID).To(Equal(bindingGUID))
			Expect(respResource.Metadata.Annotations).To(HaveKeyWithValue("foo", "bar"))
			Expect(respResource.Metadata.Labels).To(HaveKeyWithValue("baz", "bar"))
		})
	})
})
