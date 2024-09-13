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
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createDeployment), decodedDeploymentPayload)
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
				expectUnprocessableEntityError(validatorErr, "relationships is required")
			})
		})

		When("the relationship app is not specified", func() {
			BeforeEach(func() {
				createDeployment.Relationships.App = nil
			})

			It("says app is required", func() {
				expectUnprocessableEntityError(validatorErr, "app is required")
			})
		})

		When("the app guid is not specified", func() {
			BeforeEach(func() {
				createDeployment.Relationships.App.Data.GUID = ""
			})

			It("says app guid is required", func() {
				expectUnprocessableEntityError(validatorErr, "guid cannot be blank")
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

var _ = Describe("DeploymentList", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedDeploymentList payloads.DeploymentList) {
				actualDeploymentList, decodeErr := decodeQuery[payloads.DeploymentList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualDeploymentList).To(Equal(expectedDeploymentList))
			},

			Entry("app_guids", "app_guids=app_guid", payloads.DeploymentList{AppGUIDs: "app_guid"}),
			Entry("status_values ACTIVE", "status_values=ACTIVE", payloads.DeploymentList{StatusValues: "ACTIVE"}),
			Entry("status_values FINALIZED", "status_values=FINALIZED", payloads.DeploymentList{StatusValues: "FINALIZED"}),
			Entry("order_by created_at", "order_by=created_at", payloads.DeploymentList{OrderBy: "created_at"}),
			Entry("order_by -created_at", "order_by=-created_at", payloads.DeploymentList{OrderBy: "-created_at"}),
			Entry("order_by updated_at", "order_by=updated_at", payloads.DeploymentList{OrderBy: "updated_at"}),
			Entry("order_by -updated_at", "order_by=-updated_at", payloads.DeploymentList{OrderBy: "-updated_at"}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.DeploymentList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid order_by", "order_by=foo", "value must be one of"),
			Entry("invalid status_values", "status_values=foo", "value must be one of"),
		)
	})

	Describe("ToMessage", func() {
		It("translates to repository message", func() {
			deploymentList := payloads.DeploymentList{
				AppGUIDs:     "app-guid1,app-guid2",
				StatusValues: "ACTIVE,FINALIZED",
				OrderBy:      "created_at",
			}
			Expect(deploymentList.ToMessage()).To(Equal(repositories.ListDeploymentsMessage{
				AppGUIDs: []string{"app-guid1", "app-guid2"},
				StatusValues: []repositories.DeploymentStatusValue{
					repositories.DeploymentStatusValueActive,
					repositories.DeploymentStatusValueFinalized,
				},
				OrderBy: "created_at",
			}))
		})
	})
})
