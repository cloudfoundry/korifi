package payloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
)

var _ = Describe("BuildCreate", func() {
	var createPayload payloads.BuildCreate

	BeforeEach(func() {
		createPayload = payloads.BuildCreate{
			Package: &payloads.RelationshipData{
				GUID: "some-build-guid",
			},
		}
	})

	Describe("Decode", func() {
		var (
			decodedBuildPayload *payloads.BuildCreate
			validatorErr        error
		)

		BeforeEach(func() {
			decodedBuildPayload = new(payloads.BuildCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), decodedBuildPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedBuildPayload).To(gstruct.PointTo(Equal(createPayload)))
		})

		When("package is not provided", func() {
			BeforeEach(func() {
				createPayload.Package = nil
			})

			It("says package is required", func() {
				expectUnprocessableEntityError(validatorErr, "package cannot be blank")
			})
		})

		When("package guid is empty", func() {
			BeforeEach(func() {
				createPayload.Package.GUID = ""
			})

			It("says guid is required", func() {
				expectUnprocessableEntityError(validatorErr, "package.guid cannot be blank")
			})
		})

		When("the metadata annotations is not empty", func() {
			BeforeEach(func() {
				createPayload.Metadata.Annotations = map[string]string{
					"foo": "bar",
				}
			})

			It("says labels and annotations are not supported", func() {
				expectUnprocessableEntityError(validatorErr, "metadata.annotations must be blank")
			})
		})

		When("the metadata labels is not empty", func() {
			BeforeEach(func() {
				createPayload.Metadata.Labels = map[string]string{
					"foo": "bar",
				}
			})

			It("says labels and annotations are not supported", func() {
				expectUnprocessableEntityError(validatorErr, "metadata.labels must be blank")
			})
		})

		When("the lifecycle is invalid", func() {
			BeforeEach(func() {
				createPayload.Lifecycle = &payloads.Lifecycle{
					Type: "invalid",
				}
			})

			It("says lifecycle is invalid", func() {
				expectUnprocessableEntityError(validatorErr, "lifecycle.type value must be one of: buildpack, docker")
			})
		})
	})

	Describe("ToMessage", func() {
		It("translates to create build repo message", func() {
			createMessage := createPayload.ToMessage(repositories.AppRecord{
				GUID:      "guid",
				SpaceGUID: "space-guid",
				Lifecycle: repositories.Lifecycle{
					Type: "my-type",
				},
			})
			Expect(createMessage).To(Equal(repositories.CreateBuildMessage{
				AppGUID:         "guid",
				PackageGUID:     "some-build-guid",
				SpaceGUID:       "space-guid",
				StagingMemoryMB: payloads.DefaultLifecycleConfig.StagingMemoryMB,
				Lifecycle: repositories.Lifecycle{
					Type: "my-type",
				},
			}))
		})
	})
})
