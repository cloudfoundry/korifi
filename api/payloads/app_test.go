package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("AppList", func() {
	Describe("decode from url values", func() {
		It("succeeds", func() {
			var appList payloads.AppList
			req, err := http.NewRequest("GET", "http://foo.com/bar?names=name&guids=guid&space_guids=space_guid", nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &appList)

			Expect(err).NotTo(HaveOccurred())
			Expect(appList).To(Equal(payloads.AppList{
				Names:      "name",
				GUIDs:      "guid",
				SpaceGuids: "space_guid",
			}))
		})
	})
})

var _ = Describe("App payload validation", func() {
	var validatorErr error

	Describe("AppCreate", func() {
		var (
			payload        payloads.AppCreate
			decodedPayload *payloads.AppCreate
		)

		BeforeEach(func() {
			payload = payloads.AppCreate{
				Name: "my-app",
				Relationships: &payloads.AppRelationships{
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "app-guid",
						},
					},
				},
			}

			decodedPayload = new(payloads.AppCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("name is not set", func() {
			BeforeEach(func() {
				payload.Name = ""
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "name cannot be blank")
			})
		})

		When("lifecycle is invalid", func() {
			BeforeEach(func() {
				payload.Lifecycle = &payloads.Lifecycle{}
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError(validatorErr, "lifecycle.type cannot be blank")
			})
		})

		When("relationships are not set", func() {
			BeforeEach(func() {
				payload.Relationships = nil
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError(validatorErr, "relationships is required")
			})
		})

		When("relationships space is not set", func() {
			BeforeEach(func() {
				payload.Relationships.Space = nil
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError(validatorErr, "relationships.space is required")
			})
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				payload.Metadata = payloads.Metadata{
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

	Describe("AppPatch", func() {
		var (
			payload        payloads.AppPatch
			decodedPayload *payloads.AppPatch
		)

		BeforeEach(func() {
			payload = payloads.AppPatch{
				Metadata: payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"example.org/jim": tools.PtrTo("hello"),
					},
				},
			}

			decodedPayload = new(payloads.AppPatch)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
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
				expectUnprocessableEntityError(validatorErr, `Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
			})
		})
	})

	Describe("AppSetCurrentDroplet", func() {
		var (
			payload        payloads.AppSetCurrentDroplet
			decodedPayload *payloads.AppSetCurrentDroplet
		)

		BeforeEach(func() {
			payload = payloads.AppSetCurrentDroplet{
				Relationship: payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "the-guid",
					},
				},
			}

			decodedPayload = new(payloads.AppSetCurrentDroplet)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("relationship is invalid", func() {
			BeforeEach(func() {
				payload.Relationship = payloads.Relationship{}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "Relationship cannot be blank")
			})
		})
	})

	Describe("AppPatchEnvVars", func() {
		var (
			payload        payloads.AppPatchEnvVars
			decodedPayload *payloads.AppPatchEnvVars
		)

		BeforeEach(func() {
			payload = payloads.AppPatchEnvVars{
				Var: map[string]interface{}{
					"foo": "bar",
				},
			}

			decodedPayload = new(payloads.AppPatchEnvVars)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("it contains a 'PORT' key", func() {
			BeforeEach(func() {
				payload.Var["PORT"] = "2222"
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "value PORT is not allowed")
			})
		})

		When("it contains a key with prefix 'VCAP_'", func() {
			BeforeEach(func() {
				payload.Var["VCAP_foo"] = "bar"
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "prefix VCAP_ is not allowed")
			})
		})

		When("it contains a key with prefix 'VMC_'", func() {
			BeforeEach(func() {
				payload.Var["VMC_foo"] = "bar"
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "prefix VMC_ is not allowed")
			})
		})
	})
})
