package payloads_test

import (
	"encoding/json"
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ServiceInstanceList", func() {
	Describe("DecodeFromURLValues", func() {
		serviceInstanceList := payloads.ServiceInstanceList{}
		err := serviceInstanceList.DecodeFromURLValues(url.Values{
			"names":       []string{"name"},
			"space_guids": []string{"space_guid"},
			"order_by":    []string{"order"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(serviceInstanceList).To(Equal(payloads.ServiceInstanceList{
			Names:      "name",
			SpaceGuids: "space_guid",
			OrderBy:    "order",
		}))
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
		createPayload = payloads.ServiceInstanceCreate{
			Name: "service-instance-name",
			Type: "user-provided",
			Tags: []string{"foo", "bar"},
			Credentials: map[string]string{
				"username": "bob",
				"password": "float",
			},
			Relationships: payloads.ServiceInstanceRelationships{
				Space: payloads.Relationship{
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
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), serviceInstanceCreate)
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
			expectUnprocessableEntityError(validatorErr, "Name is a required field")
		})
	})

	When("type is not set", func() {
		BeforeEach(func() {
			createPayload.Type = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type is a required field")
		})
	})

	When("type is invalid", func() {
		BeforeEach(func() {
			createPayload.Type = "service-instance-type"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type must be one of [user-provided]")
		})
	})

	When("space relationship data is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.Space.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Data is a required field")
		})
	})

	When("tags length is too long", func() {
		BeforeEach(func() {
			longString := strings.Repeat("a", 2048)
			createPayload.Tags = append(createPayload.Tags, longString)
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Key: 'ServiceInstanceCreate.Tags' Error:Field validation for 'Tags' failed on the 'serviceinstancetaglength' tag")
		})
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			createPayload.Metadata = payloads.Metadata{
				Labels: map[string]string{
					"foo.cloudfoundry.org/bar": "jim",
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			createPayload.Metadata = payloads.Metadata{
				Annotations: map[string]string{
					"foo.cloudfoundry.org/bar": "jim",
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	Context("ToServiceInstanceCreateMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := serviceInstanceCreate.ToServiceInstanceCreateMessage()
			Expect(msg.Name).To(Equal("service-instance-name"))
			Expect(msg.Type).To(Equal("user-provided"))
			Expect(msg.SpaceGUID).To(Equal("space-guid"))
			Expect(msg.Tags).To(ConsistOf("foo", "bar"))
			Expect(msg.Annotations).To(HaveLen(1))
			Expect(msg.Annotations).To(HaveKeyWithValue("ann1", "val_ann1"))
			Expect(msg.Labels).To(HaveLen(1))
			Expect(msg.Labels).To(HaveKeyWithValue("lab1", "val_lab1"))
			Expect(msg.Credentials).To(HaveLen(2))
			Expect(msg.Credentials).To(HaveKeyWithValue("username", "bob"))
			Expect(msg.Credentials).To(HaveKeyWithValue("password", "float"))
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
			Credentials: &map[string]string{
				"username": "bob",
				"password": "float",
			},
			Metadata: payloads.MetadataPatch{
				Annotations: map[string]*string{"ann1": tools.PtrTo("val_ann1")},
				Labels:      map[string]*string{"lab1": tools.PtrTo("val_lab1")},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(patchPayload), serviceInstancePatch)
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
				"username": Equal("bob"),
				"password": Equal("float"),
			})))
		})
	})
})
