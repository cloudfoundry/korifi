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
				Rules: []korifiv1alpha1.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				GloballyEnabled: korifiv1alpha1.SecurityGroupWorkloads{
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
				createPayload.Rules = []korifiv1alpha1.SecurityGroupRule{}
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules cannot be blank")
			})
		})

		When("The protocol is invalid", func() {
			BeforeEach(func() {
				createPayload.Rules[0].Protocol = "invalid"
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules[0]: protocol invalid not supported")
			})
		})

		When("Protocol is ALL with ports", func() {
			BeforeEach(func() {
				createPayload.Rules[0].Protocol = korifiv1alpha1.ProtocolALL
				createPayload.Rules[0].Ports = "80"
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules[0]: ports are not allowed for protocols of type all")
			})
		})

		When("Protocol is TCP but has no ports", func() {
			BeforeEach(func() {
				createPayload.Rules[0].Protocol = korifiv1alpha1.ProtocolTCP
				createPayload.Rules[0].Ports = ""
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules[0]: ports are required for protocols of type TCP and UDP")
			})
		})

		When("Destination is invalid", func() {
			BeforeEach(func() {
				createPayload.Rules[0].Destination = "invalid-dest"
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules[0]: the destination: invalid-dest is not in a valid format")
			})
		})

		When("Ports are invalid", func() {
			BeforeEach(func() {
				createPayload.Rules[0].Ports = "invalid-port"
			})
			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "rules[0]: the ports: invalid-port is not in a valid format")
			})
		})
	})

	Describe("ToMessage", func() {
		var message repositories.CreateSecurityGroupMessage

		BeforeEach(func() {
			createPayload = payloads.SecurityGroupCreate{
				DisplayName: "test-security-group",
				Rules: []korifiv1alpha1.SecurityGroupRule{
					{Protocol: korifiv1alpha1.ProtocolTCP, Ports: "80", Destination: "192.168.1.1"},
				},
				GloballyEnabled: korifiv1alpha1.SecurityGroupWorkloads{Running: false, Staging: false},
				Relationships: payloads.SecurityGroupRelationships{
					RunningSpaces: payloads.ToManyRelationship{Data: []payloads.RelationshipData{{GUID: "space1"}}},
					StagingSpaces: payloads.ToManyRelationship{Data: []payloads.RelationshipData{{GUID: "space2"}}},
				},
			}
		})

		JustBeforeEach(func() {
			message = createPayload.ToMessage()
		})

		It("converts to repo message correctly", func() {
			Expect(message.DisplayName).To(Equal("test-security-group"))
			Expect(message.Rules).To(Equal(createPayload.Rules))
			Expect(message.GloballyEnabled).To(Equal(korifiv1alpha1.SecurityGroupWorkloads{Running: false, Staging: false}))
			Expect(message.Spaces).To(MatchAllKeys(Keys{
				"space1": Equal(korifiv1alpha1.SecurityGroupWorkloads{Running: true}),
				"space2": Equal(korifiv1alpha1.SecurityGroupWorkloads{Staging: true}),
			}))
		})
	})
})
