package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
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
		appGUID = createBuildpackApp(spaceGUID, generateGUID("app"))
		instanceGUID = createServiceInstance(spaceGUID, generateGUID("service-instance"), nil)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("POST /v3/service_credential_bindings/{guid}", func() {
		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().
				SetBody(typedResource{
					Type: "app",
					resource: resource{
						Relationships: relationships{"app": {Data: resource{GUID: appGUID}}, "service_instance": {Data: resource{GUID: instanceGUID}}},
					},
				}).
				Post("/v3/service_credential_bindings")
		})

		It("succeeds", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusCreated))
		})
	})

	Describe("GET /v3/service_credential_bindings/{guid}", func() {
		var respResource responseResource

		BeforeEach(func() {
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")
		})

		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().
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
			httpResp, httpError = adminClient.R().Delete("/v3/service_credential_bindings/" + bindingGUID)
		})

		It("succeeds", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusNoContent))
		})
	})

	Describe("GET /v3/service_credential_bindings", func() {
		var (
			anotherInstanceGUID string
			anotherBindingGUID  string
			result              resourceListWithInclusion
		)

		BeforeEach(func() {
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")

			anotherInstanceGUID = createServiceInstance(spaceGUID, generateGUID("another-service-instance"), nil)
			anotherBindingGUID = createServiceBinding(appGUID, anotherInstanceGUID, "")

			result = resourceListWithInclusion{}
		})

		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().SetResult(&result).Get("/v3/service_credential_bindings")
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

	Describe("PATCH /v3/service_credential_bindings/{guid}", func() {
		var respResource responseResource

		BeforeEach(func() {
			bindingGUID = createServiceBinding(appGUID, instanceGUID, "")
		})

		JustBeforeEach(func() {
			var err error
			httpResp, err = adminClient.R().
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
