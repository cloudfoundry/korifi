package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("PackageCreate", func() {
	var createPayload payloads.PackageCreate

	BeforeEach(func() {
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

	Describe("Validate", func() {
		var (
			packageCreate *payloads.PackageCreate
			validatorErr  error
		)

		BeforeEach(func() {
			packageCreate = new(payloads.PackageCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), packageCreate)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(packageCreate).To(gstruct.PointTo(Equal(createPayload)))
		})

		When("data is specified", func() {
			BeforeEach(func() {
				createPayload.Data = &payloads.PackageData{}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "data must be blank")
			})
		})

		When("type is docker", func() {
			BeforeEach(func() {
				createPayload.Type = "docker"
				createPayload.Data = &payloads.PackageData{
					Image: "some/image",
				}
			})

			It("succeeds", func() {
				Expect(validatorErr).NotTo(HaveOccurred())
				Expect(packageCreate).To(gstruct.PointTo(Equal(createPayload)))
			})

			When("image is not specified", func() {
				BeforeEach(func() {
					createPayload.Data = &payloads.PackageData{}
				})

				It("returns an appropriate error", func() {
					expectUnprocessableEntityError(validatorErr, "data.image cannot be blank")
				})
			})
		})

		When("type is empty", func() {
			BeforeEach(func() {
				createPayload.Type = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "type cannot be blank")
			})
		})

		When("type is not in the allowed list", func() {
			BeforeEach(func() {
				createPayload.Type = "foo"
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "type value must be one of: bits")
			})
		})

		When("relationships is not set", func() {
			BeforeEach(func() {
				createPayload.Relationships = nil
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "relationships is required")
			})
		})

		When("relationships.app is not set", func() {
			BeforeEach(func() {
				createPayload.Relationships.App = nil
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "app is required")
			})
		})

		When("relationships.app is invalid", func() {
			BeforeEach(func() {
				createPayload.Relationships.App.Data.GUID = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "guid cannot be blank")
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
	})

	Describe("ToMessage", func() {
		var createMessage repositories.CreatePackageMessage

		JustBeforeEach(func() {
			createMessage = createPayload.ToMessage(repositories.AppRecord{
				GUID:      "guid",
				SpaceGUID: "space-guid",
			})
		})

		It("create the message", func() {
			Expect(createMessage).To(Equal(repositories.CreatePackageMessage{
				Type:      "bits",
				AppGUID:   "guid",
				SpaceGUID: "space-guid",
				Metadata: repositories.Metadata{
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"example.org/jim": "hello",
					},
				},
			}))
		})

		When("package type is docker", func() {
			BeforeEach(func() {
				createPayload.Type = "docker"
				createPayload.Data = &payloads.PackageData{
					Image: "some/image",
				}
			})

			It("create the message", func() {
				Expect(createMessage).To(Equal(repositories.CreatePackageMessage{
					Type:      "docker",
					AppGUID:   "guid",
					SpaceGUID: "space-guid",
					Metadata: repositories.Metadata{
						Labels: map[string]string{
							"foo": "bar",
						},
						Annotations: map[string]string{
							"example.org/jim": "hello",
						},
					},
					Data: &repositories.PackageData{
						Image: "some/image",
					},
				}))
			})
		})
	})
})

var _ = Describe("PackageUpdate", func() {
	var payload payloads.PackageUpdate

	BeforeEach(func() {
		payload = payloads.PackageUpdate{
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

	Describe("Validation", func() {
		var (
			decodedPayload *payloads.PackageUpdate
			validatorErr   error
		)

		JustBeforeEach(func() {
			decodedPayload = new(payloads.PackageUpdate)
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				payload.Metadata = payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "label/annotation key cannot use the cloudfoundry.org domain")
			})
		})
	})

	Describe("ToMessage", func() {
		It("converts to repo message correctly", func() {
			msg := payload.ToMessage("foo")
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
