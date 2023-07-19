package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

var _ = Describe("ServiceBindingList", func() {
	decode := func(queryString string) (payloads.ServiceBindingList, error) {
		serviceBindingList := payloads.ServiceBindingList{}
		req, err := http.NewRequest("GET", "http://foo.com/bar"+queryString, nil)
		Expect(err).NotTo(HaveOccurred())

		err = validator.DecodeAndValidateURLValues(req, &serviceBindingList)
		return serviceBindingList, err
	}

	Describe("decode from url values", func() {
		var (
			queryString        string
			serviceBindingList payloads.ServiceBindingList
			decodeErr          error
		)

		BeforeEach(func() {
			queryString = "?app_guids=app_guid&service_instance_guids=service_instance_guid&include=include"
		})

		JustBeforeEach(func() {
			serviceBindingList, decodeErr = decode(queryString)
		})

		It("succeeds", func() {
			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(serviceBindingList).To(Equal(payloads.ServiceBindingList{
				AppGUIDs:             "app_guid",
				ServiceInstanceGUIDs: "service_instance_guid",
				Include:              "include",
				LabelSelector:        labels.Everything(),
			}))
		})

		When("the query string contains an invalid label selector", func() {
			BeforeEach(func() {
				queryString = "?label_selector=~~~"
			})

			It("returns an error", func() {
				Expect(decodeErr).To(MatchError(ContainSubstring("Invalid value")))
			})
		})
	})

	DescribeTable("valid label selectors",
		func(labelSelector string, verifyRequirements func(labels.Requirements)) {
			serviceBindingList := payloads.ServiceBindingList{}
			req, err := http.NewRequest("GET", "http://foo.com/bar?label_selector="+labelSelector, nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &serviceBindingList)
			Expect(err).NotTo(HaveOccurred())

			requirements, selectable := serviceBindingList.LabelSelector.Requirements()
			Expect(selectable).To(BeTrue())
			verifyRequirements(requirements)
		},
		Entry("foo exists", "foo", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.Exists))
			Expect(r[0].Values()).To(BeEmpty())
		}),
		Entry("foo does not exist", "!foo", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.DoesNotExist))
			Expect(r[0].Values()).To(BeEmpty())
		}),
		Entry("foo=bar", "foo=bar", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.Equals))
			Expect(r[0].Values()).To(SatisfyAll(HaveLen(1), HaveKey("bar")))
		}),
		Entry("foo==bar", "foo==bar", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.DoubleEquals))
			Expect(r[0].Values()).To(SatisfyAll(HaveLen(1), HaveKey("bar")))
		}),
		Entry("foo!=bar", "foo!=bar", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.NotEquals))
			Expect(r[0].Values()).To(SatisfyAll(HaveLen(1), HaveKey("bar")))
		}),
		Entry("foo in (bar1,bar2)", "foo in (bar1,bar2)", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.In))
			Expect(r[0].Values()).To(SatisfyAll(HaveLen(2), HaveKey("bar1"), HaveKey("bar2")))
		}),
		Entry("foo notin (bar1,bar2)", "foo notin (bar1,bar2)", func(r labels.Requirements) {
			Expect(r).To(HaveLen(1))
			Expect(r[0].Key()).To(Equal("foo"))
			Expect(r[0].Operator()).To(Equal(selection.NotIn))
			Expect(r[0].Values()).To(SatisfyAll(HaveLen(2), HaveKey("bar1"), HaveKey("bar2")))
		}),
	)

	Describe("ToMessage", func() {
		var (
			payload       payloads.ServiceBindingList
			message       repositories.ListServiceBindingsMessage
			labelSelector labels.Selector
		)

		BeforeEach(func() {
			fooBarRequirement, err := labels.NewRequirement("foo", selection.Equals, []string{"bar"})
			Expect(err).NotTo(HaveOccurred())
			labelSelector = labels.NewSelector().Add(*fooBarRequirement)

			payload = payloads.ServiceBindingList{
				AppGUIDs:             "app1,app2",
				ServiceInstanceGUIDs: "s1,s2",
				Include:              "include",
				LabelSelector:        labelSelector,
			}
		})

		JustBeforeEach(func() {
			message = payload.ToMessage()
		})

		It("returns a list service bindings message", func() {
			Expect(message).To(Equal(repositories.ListServiceBindingsMessage{
				AppGUIDs:             []string{"app1", "app2"},
				ServiceInstanceGUIDs: []string{"s1", "s2"},
				LabelSelector:        labelSelector,
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
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), serviceBindingCreate)
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
