package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("ServiceBindingList", func() {
	Describe("decode from url values", func() {
		It("succeeds", func() {
			serviceBindingList := payloads.ServiceBindingList{}
			req, err := http.NewRequest("GET", "http://foo.com/bar?app_guids=app_guid&service_instance_guids=service_instance_guid&include=include", nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &serviceBindingList)

			Expect(err).NotTo(HaveOccurred())
			Expect(serviceBindingList).To(Equal(payloads.ServiceBindingList{
				AppGUIDs:             "app_guid",
				ServiceInstanceGUIDs: "service_instance_guid",
				Include:              "include",
			}))
		})
	})
})

var _ = Describe("ServiceBindingCreate", func() {
	var (
		createPayload        payloads.ServiceBindingCreate
		serviceBindingCreate *payloads.ServiceBindingCreate
		validatorErr         error
		apiError             errors.ApiError
	)

	BeforeEach(func() {
		serviceBindingCreate = new(payloads.ServiceBindingCreate)
		createPayload = payloads.ServiceBindingCreate{
			Relationships: &payloads.ServiceBindingRelationships{
				App: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "app-guid",
					},
				},
				ServiceInstance: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "service-instance-guid",
					},
				},
			},
			Type: "app",
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), serviceBindingCreate)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(serviceBindingCreate).To(gstruct.PointTo(Equal(createPayload)))
	})

	When(`the type is "key"`, func() {
		BeforeEach(func() {
			createPayload.Type = "key"
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("type value must be one of: app"))
		})
	})

	When("all relationships are missing", func() {
		BeforeEach(func() {
			createPayload.Relationships = nil
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships is required"))
		})
	})

	When("app relationship is missing", func() {
		BeforeEach(func() {
			createPayload.Relationships.App = nil
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.app is required"))
		})
	})

	When("the app GUID is blank", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data.GUID = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("app.data.guid cannot be blank"))
		})
	})

	When("service instance relationship is missing", func() {
		BeforeEach(func() {
			createPayload.Relationships.ServiceInstance = nil
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.service_instance is required"))
		})
	})

	When("the service instance GUID is blank", func() {
		BeforeEach(func() {
			createPayload.Relationships.ServiceInstance.Data.GUID = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.service_instance.data.guid cannot be blank"))
		})
	})
})

var _ = Describe("ServiceBindingUpdate", func() {
	var (
		patchPayload        payloads.ServiceBindingUpdate
		serviceBindingPatch *payloads.ServiceBindingUpdate
		validatorErr        error
		apiError            errors.ApiError
	)

	BeforeEach(func() {
		serviceBindingPatch = new(payloads.ServiceBindingUpdate)
		patchPayload = payloads.ServiceBindingUpdate{
			Metadata: payloads.MetadataPatch{
				Annotations: map[string]*string{"a": tools.PtrTo("av")},
				Labels:      map[string]*string{"l": tools.PtrTo("lv")},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(patchPayload), serviceBindingPatch)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(serviceBindingPatch).To(gstruct.PointTo(Equal(patchPayload)))
	})

	When("metadata uses the cloudfoundry domain", func() {
		BeforeEach(func() {
			patchPayload.Metadata.Labels["foo.cloudfoundry.org/bar"] = tools.PtrTo("baz")
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("cannot use the cloudfoundry.org domain"))
		})
	})
})
