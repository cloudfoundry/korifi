package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"

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
			Relationships: payloads.DeploymentRelationships{
				App: payloads.Relationship{
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
			body, err := json.Marshal(createDeployment)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("", "", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedDeploymentPayload)
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

		When("the app relationship is not specified", func() {
			BeforeEach(func() {
				createDeployment.Relationships = payloads.DeploymentRelationships{}
			})

			It("says app data is required", func() {
				expectUnprocessableEntityError(validatorErr, "Data is a required field")
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
