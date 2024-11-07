package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

var _ = Describe("ServiceBindingList", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceBindingList payloads.ServiceBindingList) {
			actualServiceBindingList, decodeErr := decodeQuery[payloads.ServiceBindingList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceBindingList).To(Equal(expectedServiceBindingList))
		},
		Entry("type", "type=key", payloads.ServiceBindingList{Type: korifiv1alpha1.CFServiceBindingTypeKey}),
		Entry("app_guids", "app_guids=app_guid", payloads.ServiceBindingList{AppGUIDs: "app_guid"}),
		Entry("service_instance_guids", "service_instance_guids=si_guid", payloads.ServiceBindingList{ServiceInstanceGUIDs: "si_guid"}),
		Entry("include", "include=app", payloads.ServiceBindingList{Include: "app"}),
		Entry("label_selector=foo", "label_selector=foo", payloads.ServiceBindingList{LabelSelector: "foo"}),
		Entry("service_plan_guids=plan-guid", "service_plan_guids=plan-guid", payloads.ServiceBindingList{PlanGUIDs: "plan-guid"}),
	)

	DescribeTable("invalid query",
		func(query string, errMatcher types.GomegaMatcher) {
			_, decodeErr := decodeQuery[payloads.ServiceBindingList](query)
			Expect(decodeErr).To(errMatcher)
		},
		Entry("invalid type", "type=foo", MatchError(ContainSubstring("value must be one of"))),
		Entry("invalid include type", "include=foo", MatchError(ContainSubstring("value must be one of"))),
	)

	Describe("ToMessage", func() {
		var (
			payload payloads.ServiceBindingList
			message repositories.ListServiceBindingsMessage
		)

		BeforeEach(func() {
			payload = payloads.ServiceBindingList{
				Type:                 korifiv1alpha1.CFServiceBindingTypeApp,
				AppGUIDs:             "app1,app2",
				ServiceInstanceGUIDs: "s1,s2",
				Include:              "include",
				LabelSelector:        "foo=bar",
				PlanGUIDs:            "p1,p2",
			}
		})

		JustBeforeEach(func() {
			message = payload.ToMessage()
		})

		It("returns a list service bindings message", func() {
			Expect(message).To(Equal(repositories.ListServiceBindingsMessage{
				Type:                 tools.PtrTo(korifiv1alpha1.CFServiceBindingTypeApp),
				AppGUIDs:             []string{"app1", "app2"},
				ServiceInstanceGUIDs: []string{"s1", "s2"},
				LabelSelector:        "foo=bar",
				PlanGUIDs:            []string{"p1", "p2"},
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

	When("binding is of type key", func() {
		BeforeEach(func() {
			serviceBindingCreate = new(payloads.ServiceBindingCreate)
			createPayload = payloads.ServiceBindingCreate{
				Relationships: &payloads.ServiceBindingRelationships{
					ServiceInstance: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "service-instance-guid",
						},
					},
				},
				Type: "key",
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), serviceBindingCreate)
			apiError, _ = validatorErr.(errors.ApiError)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(serviceBindingCreate).To(gstruct.PointTo(Equal(createPayload)))
		})
	})

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
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), serviceBindingCreate)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(serviceBindingCreate).To(gstruct.PointTo(Equal(createPayload)))
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
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(patchPayload), serviceBindingPatch)
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
