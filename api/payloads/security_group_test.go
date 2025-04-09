package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("SecurityGroupCreate", func() {
	var (
		createPayload       payloads.SecurityGroupCreate
		securityGroupCreate *payloads.SecurityGroupCreate
		validatorErr        error
	)

	BeforeEach(func() {
		securityGroupCreate = new(payloads.SecurityGroupCreate)
	})

	Describe("Validation", func() {
		BeforeEach(func() {
			createPayload = payloads.SecurityGroupCreate{
				DisplayName: "test-security-group",
				Rules: []payloads.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				GloballyEnabled: payloads.SecurityGroupWorkloads{
					Running: false,
					Staging: false,
				},
				Relationships: payloads.SecurityGroupRelationships{
					RunningSpaces: payloads.ToManyRelationship{Data: []payloads.RelationshipData{{GUID: "space1"}}},
					StagingSpaces: payloads.ToManyRelationship{Data: []payloads.RelationshipData{{GUID: "space2"}}},
				},
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), securityGroupCreate)
		})

		It("succeeds with valid payload", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(securityGroupCreate).To(PointTo(Equal(createPayload)))
		})

		When("The display name is empty", func() {
			BeforeEach(func() {
				createPayload.DisplayName = ""
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "name cannot be blank")
			})
		})

		When("The rules are empty", func() {
			BeforeEach(func() {
				createPayload.Rules = []payloads.SecurityGroupRule{}
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules cannot be blank")
			})
		})

		When("Converting to repo message", func() {
			var message repositories.CreateSecurityGroupMessage

			BeforeEach(func() {
				message = createPayload.ToMessage()
			})

			It("Converts the message correctly", func() {
				Expect(message.DisplayName).To(Equal("test-security-group"))
				Expect(message.Rules).To(Equal([]repositories.SecurityGroupRule{{
					Protocol:    korifiv1alpha1.ProtocolTCP,
					Ports:       "80",
					Destination: "192.168.1.1",
				}}))
				Expect(message.GloballyEnabled).To(Equal(repositories.SecurityGroupWorkloads{Running: false, Staging: false}))
				Expect(message.Spaces).To(MatchAllKeys(Keys{
					"space1": Equal(repositories.SecurityGroupWorkloads{Running: true}),
					"space2": Equal(repositories.SecurityGroupWorkloads{Staging: true}),
				}))
			})
		})
	})
})
