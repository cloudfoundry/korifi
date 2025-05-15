package payloads_test

import (
	"encoding/json"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ServiceInstanceList", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceInstanceList payloads.ServiceInstanceList) {
			actualServiceInstanceList, decodeErr := decodeQuery[payloads.ServiceInstanceList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceInstanceList).To(Equal(expectedServiceInstanceList))
		},
		Entry("names", "names=name", payloads.ServiceInstanceList{Names: "name"}),
		Entry("space_guids", "space_guids=space_guid", payloads.ServiceInstanceList{SpaceGUIDs: "space_guid"}),
		Entry("guids", "guids=guid", payloads.ServiceInstanceList{GUIDs: "guid"}),
		Entry("type", "type=managed", payloads.ServiceInstanceList{Type: "managed"}),
		Entry("created_at", "order_by=created_at", payloads.ServiceInstanceList{OrderBy: "created_at"}),
		Entry("-created_at", "order_by=-created_at", payloads.ServiceInstanceList{OrderBy: "-created_at"}),
		Entry("updated_at", "order_by=updated_at", payloads.ServiceInstanceList{OrderBy: "updated_at"}),
		Entry("-updated_at", "order_by=-updated_at", payloads.ServiceInstanceList{OrderBy: "-updated_at"}),
		Entry("name", "order_by=name", payloads.ServiceInstanceList{OrderBy: "name"}),
		Entry("-name", "order_by=-name", payloads.ServiceInstanceList{OrderBy: "-name"}),
		Entry("fields[service_plan.service_offering.service_broker]",
			"fields[service_plan.service_offering.service_broker]=guid,name",
			payloads.ServiceInstanceList{IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"service_plan", "service_offering", "service_broker"},
				Fields:           []string{"guid", "name"},
			}}}),
		Entry("fields[service_plan.service_offering]",
			"fields[service_plan.service_offering]=guid,name,relationships.service_broker,description,tags,documentation_url",
			payloads.ServiceInstanceList{IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"service_plan", "service_offering"},
				Fields:           []string{"guid", "name", "relationships.service_broker", "description", "tags", "documentation_url"},
			}}}),

		Entry("fields[service_plan]",
			"fields[service_plan]=guid,name,relationships.service_offering",
			payloads.ServiceInstanceList{IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"service_plan"},
				Fields:           []string{"guid", "name", "relationships.service_offering"},
			}}}),
		Entry("fields[space]",
			"fields[space]=guid,name,relationships.organization",
			payloads.ServiceInstanceList{IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"space"},
				Fields:           []string{"guid", "name", "relationships.organization"},
			}}}),
		Entry("fields[space.organization]",
			"fields[space.organization]=guid,name",
			payloads.ServiceInstanceList{IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"space", "organization"},
				Fields:           []string{"guid", "name"},
			}}}),
		Entry("label_selector=foo", "label_selector=foo", payloads.ServiceInstanceList{LabelSelector: "foo"}),
		Entry("service_plan_guids=plan-guid", "service_plan_guids=plan-guid", payloads.ServiceInstanceList{PlanGUIDs: "plan-guid"}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.ServiceInstanceList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid type", "type=foo", "value must be one of"),
		Entry("invalid order_by", "order_by=foo", "value must be one of"),
		Entry("invalid fields", "fields[foo]=bar", "unsupported query parameter: fields[foo]"),
		Entry("invalid service offering fields", "fields[service_plan.service_offering]=foo", "value must be one of"),
		Entry("invalid service broker fields", "fields[service_plan.service_offering.service_broker]=foo", "value must be one of"),
		Entry("invalid service plan fields", "fields[service_plan]=foo", "value must be one of"),
		Entry("invalid space fields", "fields[space]=foo", "value must be one of"),
		Entry("invalid organization fields", "fields[space.organization]=foo", "value must be one of"),
	)

	Describe("ToMessage", func() {
		var (
			payload payloads.ServiceInstanceList
			message repositories.ListServiceInstanceMessage
		)

		BeforeEach(func() {
			payload = payloads.ServiceInstanceList{
				Names:         "n1,n2",
				GUIDs:         "g1,g2",
				SpaceGUIDs:    "sg1,sg2",
				Type:          "managed",
				OrderBy:       "order",
				LabelSelector: "foo=bar",
				PlanGUIDs:     "p1,p2",
			}
		})

		JustBeforeEach(func() {
			message = payload.ToMessage()
		})

		It("returns a list service instances message", func() {
			Expect(message).To(Equal(repositories.ListServiceInstanceMessage{
				Names:         []string{"n1", "n2"},
				SpaceGUIDs:    []string{"sg1", "sg2"},
				GUIDs:         []string{"g1", "g2"},
				Type:          "managed",
				OrderBy:       "order",
				LabelSelector: "foo=bar",
				PlanGUIDs:     []string{"p1", "p2"},
			}))
		})
	})

	_ = Describe("ServiceInstanceGet", func() {
		DescribeTable("valid query",
			func(query string, expectedServiceInstanceGet payloads.ServiceInstanceGet) {
				actualServiceInstanceGet, decodeErr := decodeQuery[payloads.ServiceInstanceGet](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualServiceInstanceGet).To(Equal(expectedServiceInstanceGet))
			},
			Entry("fields[service_plan.service_offering.service_broker]",
				"fields[service_plan.service_offering.service_broker]=guid,name",
				payloads.ServiceInstanceGet{IncludeResourceRules: []params.IncludeResourceRule{{
					RelationshipPath: []string{"service_plan", "service_offering", "service_broker"},
					Fields:           []string{"guid", "name"},
				}}}),
			Entry("fields[service_plan.service_offering]",
				"fields[service_plan.service_offering]=guid,name,relationships.service_broker",
				payloads.ServiceInstanceGet{IncludeResourceRules: []params.IncludeResourceRule{{
					RelationshipPath: []string{"service_plan", "service_offering"},
					Fields:           []string{"guid", "name", "relationships.service_broker"},
				}}}),

			Entry("fields[service_plan]",
				"fields[service_plan]=guid,name,relationships.service_offering",
				payloads.ServiceInstanceGet{IncludeResourceRules: []params.IncludeResourceRule{{
					RelationshipPath: []string{"service_plan"},
					Fields:           []string{"guid", "name", "relationships.service_offering"},
				}}}),
			Entry("fields[space]",
				"fields[space]=guid,name",
				payloads.ServiceInstanceGet{IncludeResourceRules: []params.IncludeResourceRule{{
					RelationshipPath: []string{"space"},
					Fields:           []string{"guid", "name"},
				}}}),
			Entry("fields[space.organization]",
				"fields[space.organization]=guid,name",
				payloads.ServiceInstanceGet{IncludeResourceRules: []params.IncludeResourceRule{{
					RelationshipPath: []string{"space", "organization"},
					Fields:           []string{"guid", "name"},
				}}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.ServiceInstanceGet](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid service offering fields", "fields[service_plan.service_offering]=foo", "value must be one of"),
			Entry("invalid service broker fields", "fields[service_plan.service_offering.service_broker]=foo", "value must be one of"),
			Entry("invalid service plan fields", "fields[service_plan]=foo", "value must be one of"),
			Entry("invalid space fields", "fields[space]=foo", "value must be one of"),
			Entry("invalid organization fields", "fields[space.organization]=foo", "value must be one of"),
		)
	})
})

var _ = Describe("ServiceInstanceCreate", func() {
	var (
		createPayload         payloads.ServiceInstanceCreate
		serviceInstanceCreate *payloads.ServiceInstanceCreate
		validatorErr          error
	)

	BeforeEach(func() {
		serviceInstanceCreate = new(payloads.ServiceInstanceCreate)
	})

	Describe("Validation", func() {
		BeforeEach(func() {
			createPayload = payloads.ServiceInstanceCreate{
				Name: "service-instance-name",
				Type: "user-provided",
				Tags: []string{"foo", "bar"},
				Credentials: map[string]any{
					"username": "bob",
					"password": "float",
					"object": map[string]any{
						"a": "b",
					},
				},
				Relationships: &payloads.ServiceInstanceRelationships{
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "space-guid",
						},
					},
				},
				Metadata: payloads.Metadata{
					Annotations: map[string]string{"ann1": "val_ann1"},
					Labels:      map[string]string{"lab1": "val_lab1"},
				},
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), serviceInstanceCreate)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(serviceInstanceCreate).To(PointTo(Equal(createPayload)))
		})

		When("name is not set", func() {
			BeforeEach(func() {
				createPayload.Name = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "name cannot be blank")
			})
		})

		When("type is not set", func() {
			BeforeEach(func() {
				createPayload.Type = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "type cannot be blank")
			})
		})

		When("type is invalid", func() {
			BeforeEach(func() {
				createPayload.Type = "service-instance-type"
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "type value must be one of: user-provided")
			})
		})

		When("space relationship data is not set", func() {
			BeforeEach(func() {
				createPayload.Relationships.Space.Data = nil
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "data is required")
			})
		})

		When("tags length is too long", func() {
			BeforeEach(func() {
				longString := strings.Repeat("a", 2048)
				createPayload.Tags = append(createPayload.Tags, longString)
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "combined length of tags cannot exceed")
			})
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				createPayload.Metadata = payloads.Metadata{
					Labels: map[string]string{
						"foo.cloudfoundry.org/bar": "jim",
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "label/annotation key cannot use the cloudfoundry.org domain")
			})
		})

		When("the instance type is managed", func() {
			BeforeEach(func() {
				createPayload.Type = "managed"
				createPayload.Credentials = nil
				createPayload.Relationships.ServicePlan = &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "plan_guid",
					},
				}
			})

			It("succeeds", func() {
				Expect(validatorErr).NotTo(HaveOccurred())
				Expect(serviceInstanceCreate).To(PointTo(Equal(createPayload)))
			})

			When("plan relationship is not set", func() {
				BeforeEach(func() {
					createPayload.Relationships.ServicePlan = nil
				})

				It("return an appropriate error", func() {
					expectUnprocessableEntityError(validatorErr, "relationships.service_plan is required")
				})
			})
		})
	})

	Describe("ToUPSICreateMessage()", func() {
		var msg repositories.CreateUPSIMessage

		BeforeEach(func() {
			createPayload = payloads.ServiceInstanceCreate{
				Name: "service-instance-name",
				Type: "user-provided",
				Tags: []string{"foo", "bar"},
				Credentials: map[string]any{
					"username": "bob",
					"password": "float",
					"object": map[string]any{
						"a": "b",
					},
				},
				Relationships: &payloads.ServiceInstanceRelationships{
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "space-guid",
						},
					},
				},
				Metadata: payloads.Metadata{
					Annotations: map[string]string{"ann1": "val_ann1"},
					Labels:      map[string]string{"lab1": "val_lab1"},
				},
			}
		})

		JustBeforeEach(func() {
			msg = createPayload.ToUPSICreateMessage()
		})

		It("converts to repo message correctly", func() {
			Expect(msg.Name).To(Equal("service-instance-name"))
			Expect(msg.SpaceGUID).To(Equal("space-guid"))
			Expect(msg.Tags).To(ConsistOf("foo", "bar"))
			Expect(msg.Annotations).To(HaveLen(1))
			Expect(msg.Annotations).To(HaveKeyWithValue("ann1", "val_ann1"))
			Expect(msg.Labels).To(HaveLen(1))
			Expect(msg.Labels).To(HaveKeyWithValue("lab1", "val_lab1"))
			Expect(msg.Credentials).To(MatchAllKeys(Keys{
				"username": Equal("bob"),
				"password": Equal("float"),
				"object": MatchAllKeys(Keys{
					"a": Equal("b"),
				}),
			}))
		})
	})

	Describe("ToManagedSICreateMessage()", func() {
		var msg repositories.CreateManagedSIMessage

		BeforeEach(func() {
			createPayload = payloads.ServiceInstanceCreate{
				Name: "service-instance-name",
				Type: "managed",
				Tags: []string{"foo", "bar"},
				Parameters: map[string]any{
					"param1": "param1-value",
				},
				Relationships: &payloads.ServiceInstanceRelationships{
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "space-guid",
						},
					},
					ServicePlan: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "plan-guid",
						},
					},
				},
				Metadata: payloads.Metadata{
					Annotations: map[string]string{"ann1": "val_ann1"},
					Labels:      map[string]string{"lab1": "val_lab1"},
				},
			}
		})

		JustBeforeEach(func() {
			msg = createPayload.ToManagedSICreateMessage()
		})

		It("converts to repo message correctly", func() {
			Expect(msg).To(Equal(repositories.CreateManagedSIMessage{
				Name:      "service-instance-name",
				SpaceGUID: "space-guid",
				PlanGUID:  "plan-guid",
				Parameters: map[string]any{
					"param1": "param1-value",
				},
				Tags:        []string{"foo", "bar"},
				Labels:      map[string]string{"lab1": "val_lab1"},
				Annotations: map[string]string{"ann1": "val_ann1"},
			}))
		})
	})
})

var _ = Describe("ServiceInstancePatch custom unmarshalling", func() {
	var (
		payload string
		patch   payloads.ServiceInstancePatch
	)

	BeforeEach(func() {
		patch = payloads.ServiceInstancePatch{}
		payload = `{
			"name": "bob",
			"tags": ["foo", "bar"],
			"credentials": {"username": "password"},
			"metadata": {
				"labels": {"l1": "l1v"},
				"annotations": {"a1": "a1v"}
			}
		}`
	})

	JustBeforeEach(func() {
		err := json.Unmarshal([]byte(payload), &patch)
		Expect(err).NotTo(HaveOccurred())
	})

	It("sets the fields correctly", func() {
		Expect(patch.Name).To(Equal(tools.PtrTo("bob")))
		Expect(patch.Tags).To(PointTo(ConsistOf("foo", "bar")))
	})

	When("tags and credentials are not present", func() {
		BeforeEach(func() {
			payload = `{}`
		})

		It("has nil pointers for slice and map fields", func() {
			Expect(patch.Tags).To(BeNil())
			Expect(patch.Credentials).To(BeNil())
		})
	})

	When("tags and credentials are present but null", func() {
		BeforeEach(func() {
			payload = `{"tags": null, "credentials": null}`
		})

		It("defaults them to empty slice/maps", func() {
			Expect(patch.Tags).ToNot(BeNil())
			Expect(patch.Tags).To(PointTo(HaveLen(0)))
			Expect(patch.Credentials).ToNot(BeNil())
			Expect(patch.Credentials).To(PointTo(HaveLen(0)))
		})
	})
})

var _ = Describe("ServiceInstancePatch", func() {
	var (
		patchPayload         payloads.ServiceInstancePatch
		serviceInstancePatch *payloads.ServiceInstancePatch
		validatorErr         error
	)

	BeforeEach(func() {
		serviceInstancePatch = new(payloads.ServiceInstancePatch)
		patchPayload = payloads.ServiceInstancePatch{
			Name: tools.PtrTo("service-instance-name"),
			Tags: &[]string{"foo", "bar"},
			Credentials: &map[string]any{
				"object": map[string]any{
					"a": "b",
				},
			},
			Metadata: payloads.MetadataPatch{
				Annotations: map[string]*string{"ann1": tools.PtrTo("val_ann1")},
				Labels:      map[string]*string{"lab1": tools.PtrTo("val_lab1")},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(patchPayload), serviceInstancePatch)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(serviceInstancePatch).To(PointTo(Equal(patchPayload)))
	})

	When("nothing is set", func() {
		BeforeEach(func() {
			patchPayload = payloads.ServiceInstancePatch{}
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(serviceInstancePatch).To(PointTo(Equal(patchPayload)))
		})
	})

	When("metadata is invalid", func() {
		BeforeEach(func() {
			patchPayload.Metadata.Labels["foo.cloudfoundry.org/bar"] = tools.PtrTo("baz")
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "label/annotation key cannot use the cloudfoundry.org domain")
		})
	})

	Context("ToServiceInstancePatchMessage", func() {
		It("converts to repo message correctly", func() {
			msg := serviceInstancePatch.ToServiceInstancePatchMessage("space-guid", "app-guid")
			Expect(msg.SpaceGUID).To(Equal("space-guid"))
			Expect(msg.GUID).To(Equal("app-guid"))
			Expect(msg.Name).To(PointTo(Equal("service-instance-name")))
			Expect(msg.Tags).To(PointTo(ConsistOf("foo", "bar")))
			Expect(msg.Annotations).To(MatchAllKeys(Keys{
				"ann1": PointTo(Equal("val_ann1")),
			}))
			Expect(msg.Labels).To(MatchAllKeys(Keys{
				"lab1": PointTo(Equal("val_lab1")),
			}))
			Expect(msg.Credentials).To(PointTo(MatchAllKeys(Keys{
				"object": MatchAllKeys(Keys{
					"a": Equal("b"),
				}),
			})))
		})
	})
})

var _ = Describe("ServiceInstanceDelete", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceInstanceDelete payloads.ServiceInstanceDelete) {
			actualServiceInstanceDelete, decodeErr := decodeQuery[payloads.ServiceInstanceDelete](query)

			Expect(decodeErr).ToNot(HaveOccurred())
			Expect(*actualServiceInstanceDelete).To(Equal(expectedServiceInstanceDelete))
		},
		Entry("purge", "purge=true", payloads.ServiceInstanceDelete{Purge: true}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.ServiceInstanceDelete](query)
			Expect(decodeErr).To(HaveOccurred())
		},
		Entry("unsuported param", "foo=bar", "unsupported query parameter: foo"),
		Entry("invalid value for purge", "purge=foo", "invalid syntax"),
	)
})
