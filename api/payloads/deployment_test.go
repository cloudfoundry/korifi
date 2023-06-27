package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/onsi/gomega/gstruct"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeploymentCreate", func() {
	var createDeployment payloads.DeploymentCreate

	BeforeEach(func() {
		createDeployment = payloads.DeploymentCreate{
			Droplet: payloads.DropletGUID{
				Guid: "the-droplet",
			},
			Relationships: &payloads.DeploymentRelationships{
				App: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "the-app",
					},
				},
			},
		}
	})

	Describe("Decode", func() {
		var (
			decodedDeploymentPayload *payloads.DeploymentCreate
			validatorErr             error
		)

		BeforeEach(func() {
			decodedDeploymentPayload = new(payloads.DeploymentCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createDeployment), decodedDeploymentPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedDeploymentPayload).To(gstruct.PointTo(Equal(createDeployment)))
		})

		When("droplet is not specified", func() {
			BeforeEach(func() {
				createDeployment.Droplet = payloads.DropletGUID{}
			})

			It("succeeds", func() {
				Expect(validatorErr).NotTo(HaveOccurred())
				Expect(decodedDeploymentPayload).To(gstruct.PointTo(Equal(createDeployment)))
			})
		})

		When("the relationship is not specified", func() {
			BeforeEach(func() {
				createDeployment.Relationships = nil
			})

			It("says relationships is required", func() {
				expectUnprocessableEntityError(validatorErr, "Relationships is a required field")
			})
		})

		When("the relationship app is not specified", func() {
			BeforeEach(func() {
				createDeployment.Relationships.App = nil
			})

			It("says app is required", func() {
				expectUnprocessableEntityError(validatorErr, "App is a required field")
			})
		})

		When("the app guid is not specified", func() {
			BeforeEach(func() {
				createDeployment.Relationships.App.Data.GUID = ""
			})

			It("says app guid is required", func() {
				expectUnprocessableEntityError(validatorErr, "GUID is a required field")
			})
		})
	})

	Describe("ToMessage", func() {
		var createMessage repositories.CreateDeploymentMessage

		JustBeforeEach(func() {
			createMessage = createDeployment.ToMessage()
		})

		It("returns a deployment create message", func() {
			Expect(createMessage).To(Equal(repositories.CreateDeploymentMessage{
				AppGUID:     "the-app",
				DropletGUID: "the-droplet",
			}))
		})
	})
})
