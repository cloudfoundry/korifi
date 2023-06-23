package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("PackageCreate", func() {
	var (
		createPayload payloads.PackageCreate
		packageCreate *payloads.PackageCreate
		validatorErr  error
	)

	BeforeEach(func() {
		packageCreate = new(payloads.PackageCreate)
		createPayload = payloads.PackageCreate{
			Type: "bits",
			Relationships: &payloads.PackageRelationships{
				App: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "some-guid",
					},
				},
			},
			Metadata: payloads.Metadata{
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"example.org/jim": "hello",
				},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), packageCreate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(packageCreate).To(gstruct.PointTo(Equal(createPayload)))
	})

	When("type is empty", func() {
		BeforeEach(func() {
			createPayload.Type = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type is a required field")
		})
	})

	When("type is not in the allowed list", func() {
		BeforeEach(func() {
			createPayload.Type = "foo"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type must be one of ['bits']")
		})
	})

	When("relationships is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Relationships is a required field")
		})
	})

	When("relationships.app is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "App is a required field")
		})
	})

	When("relationships.app.data is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Data is a required field")
		})
	})

	When("relationships.app.data.guid is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data.GUID = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "GUID is a required field")
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
})

var _ = Describe("PackageUpdate", func() {
	var (
		updatePayload payloads.PackageUpdate
		packageUpdate *payloads.PackageUpdate
		validatorErr  error
	)

	BeforeEach(func() {
		packageUpdate = new(payloads.PackageUpdate)
		updatePayload = payloads.PackageUpdate{
			Metadata: payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo": tools.PtrTo("bar"),
					"bar": nil,
				},
				Annotations: map[string]*string{
					"example.org/jim": tools.PtrTo("hello"),
				},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(updatePayload), packageUpdate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(packageUpdate).To(gstruct.PointTo(Equal(updatePayload)))
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Annotations: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	Context("toMessage", func() {
		It("converts to repo message correctly", func() {
			msg := packageUpdate.ToMessage("foo")
			Expect(msg.MetadataPatch.Labels).To(Equal(map[string]*string{
				"foo": tools.PtrTo("bar"),
				"bar": nil,
			}))
		})
	})
})

var _ = Describe("PackageList", func() {
	DescribeTable("valid query",
		func(query string, expectedPackageListQueryParameters payloads.PackageList) {
			actualPackageListQueryParameters, decodeErr := decodeQuery[payloads.PackageList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualPackageListQueryParameters).To(Equal(expectedPackageListQueryParameters))
		},
		Entry("app_guids", "app_guids=g1,g2", payloads.PackageList{AppGUIDs: "g1,g2"}),
		Entry("states", "states=s1,s2", payloads.PackageList{States: "s1,s2"}),
		Entry("created_at", "order_by=created_at", payloads.PackageList{OrderBy: "created_at"}),
		Entry("-created_at", "order_by=-created_at", payloads.PackageList{OrderBy: "-created_at"}),
		Entry("updated_at", "order_by=updated_at", payloads.PackageList{OrderBy: "updated_at"}),
		Entry("-updated_at", "order_by=-updated_at", payloads.PackageList{OrderBy: "-updated_at"}),
		Entry("empty", "order_by=", payloads.PackageList{OrderBy: ""}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.PackageList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid order_by", "order_by=foo", "value must be one of"),
	)
})
