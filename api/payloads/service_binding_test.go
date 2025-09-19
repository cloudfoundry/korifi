package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
		Entry("type", "type=app", payloads.ServiceBindingList{Type: korifiv1alpha1.CFServiceBindingTypeApp}),
		Entry("names", "names=name1,name2", payloads.ServiceBindingList{Names: "name1,name2"}),
		Entry("app_guids", "app_guids=app_guid", payloads.ServiceBindingList{AppGUIDs: "app_guid"}),
		Entry("service_instance_guids", "service_instance_guids=si_guid", payloads.ServiceBindingList{ServiceInstanceGUIDs: "si_guid"}),
		Entry("include", "include=app", payloads.ServiceBindingList{Include: "app"}),
		Entry("include", "include=service_instance", payloads.ServiceBindingList{Include: "service_instance"}),
		Entry("label_selector=foo", "label_selector=foo", payloads.ServiceBindingList{LabelSelector: "foo"}),
		Entry("service_plan_guids=plan-guid", "service_plan_guids=plan-guid", payloads.ServiceBindingList{PlanGUIDs: "plan-guid"}),
		Entry("order_by created_at", "order_by=created_at", payloads.ServiceBindingList{OrderBy: "created_at"}),
		Entry("order_by -created_at", "order_by=-created_at", payloads.ServiceBindingList{OrderBy: "-created_at"}),
		Entry("order_by updated_at", "order_by=updated_at", payloads.ServiceBindingList{OrderBy: "updated_at"}),
		Entry("order_by -updated_at", "order_by=-updated_at", payloads.ServiceBindingList{OrderBy: "-updated_at"}),
		Entry("order_by name", "order_by=name", payloads.ServiceBindingList{OrderBy: "name"}),
		Entry("order_by -name", "order_by=-name", payloads.ServiceBindingList{OrderBy: "-name"}),
		Entry("page=3", "page=3", payloads.ServiceBindingList{Pagination: payloads.Pagination{Page: "3"}}),
	)

	DescribeTable("invalid query",
		func(query string, errMatcher types.GomegaMatcher) {
			_, decodeErr := decodeQuery[payloads.ServiceBindingList](query)
			Expect(decodeErr).To(errMatcher)
		},
		Entry("invalid type", "type=foo", MatchError(ContainSubstring("value must be one of"))),
		Entry("invalid include type", "include=foo", MatchError(ContainSubstring("value must be one of"))),
		Entry("invalid order_by", "order_by=foo", MatchError(ContainSubstring("value must be one of"))),
		Entry("per_page is not a number", "per_page=foo", MatchError(ContainSubstring("value must be an integer"))),
	)

	Describe("ToMessage", func() {
		var (
			payload payloads.ServiceBindingList
			message repositories.ListServiceBindingsMessage
		)

		BeforeEach(func() {
			payload = payloads.ServiceBindingList{
				Type:                 korifiv1alpha1.CFServiceBindingTypeApp,
				Names:                "binding1,binding2",
				AppGUIDs:             "app1,app2",
				ServiceInstanceGUIDs: "s1,s2",
				Include:              "include",
				LabelSelector:        "foo=bar",
				PlanGUIDs:            "p1,p2",
				OrderBy:              "foo",
				Pagination: payloads.Pagination{
					Page:    "1",
					PerPage: "20",
				},
			}
		})

		JustBeforeEach(func() {
			message = payload.ToMessage()
		})

		It("returns a list service bindings message", func() {
			Expect(message).To(Equal(repositories.ListServiceBindingsMessage{
				Type:                 korifiv1alpha1.CFServiceBindingTypeApp,
				AppGUIDs:             []string{"app1", "app2"},
				Names:                []string{"binding1", "binding2"},
				ServiceInstanceGUIDs: []string{"s1", "s2"},
				LabelSelector:        "foo=bar",
				PlanGUIDs:            []string{"p1", "p2"},
				OrderBy:              "foo",
				Pagination: repositories.Pagination{
					Page:    1,
					PerPage: 20,
				},
			}))
		})
	})
})

var _ = Describe("ServiceBindingCreate", func() {
	var createPayload payloads.ServiceBindingCreate

	BeforeEach(func() {
		createPayload = payloads.ServiceBindingCreate{
			Name: tools.PtrTo(uuid.NewString()),
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
			Parameters: map[string]any{
				"p1": "p1-value",
			},
		}
	})

	Describe("Validation", func() {
		var (
			serviceBindingCreate *payloads.ServiceBindingCreate
			validatorErr         error
			apiError             errors.ApiError
		)

		BeforeEach(func() {
			serviceBindingCreate = new(payloads.ServiceBindingCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), serviceBindingCreate)
			apiError, _ = validatorErr.(errors.ApiError)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(serviceBindingCreate).To(PointTo(Equal(createPayload)))
		})

		When("name is omitted", func() {
			BeforeEach(func() {
				createPayload.Name = nil
			})

			It("succeeds", func() {
				Expect(validatorErr).NotTo(HaveOccurred())
				Expect(serviceBindingCreate).To(PointTo(Equal(createPayload)))
			})
		})

		When(`the type is "key"`, func() {
			BeforeEach(func() {
				createPayload.Type = "key"
			})

			It("succeeds", func() {
				Expect(validatorErr).NotTo(HaveOccurred())
				Expect(serviceBindingCreate).To(PointTo(Equal(createPayload)))
			})

			When("name field is omitted", func() {
				BeforeEach(func() {
					createPayload.Name = nil
				})

				It("fails validation", func() {
					Expect(apiError).To(HaveOccurred())
					Expect(apiError.Detail()).To(ContainSubstring("name cannot be blank"))
				})
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

	Describe("ToMessage", func() {
		var createMessage repositories.CreateServiceBindingMessage

		JustBeforeEach(func() {
			createMessage = createPayload.ToMessage("space-guid")
		})

		It("creates the message", func() {
			Expect(createMessage).To(Equal(repositories.CreateServiceBindingMessage{
				Name:                createPayload.Name,
				ServiceInstanceGUID: createPayload.Relationships.ServiceInstance.Data.GUID,
				AppGUID:             createPayload.Relationships.App.Data.GUID,
				SpaceGUID:           "space-guid",
				Type:                "app",
				Parameters: map[string]any{
					"p1": "p1-value",
				},
			}))
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
		Expect(serviceBindingPatch).To(PointTo(Equal(patchPayload)))
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
