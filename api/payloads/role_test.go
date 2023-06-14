package payloads_test

import (
	"bytes"
	"fmt"
	"net/http"

	rbacv1 "k8s.io/api/rbac/v1"

	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("RoleCreate", func() {
	var (
		createPayload payloads.RoleCreate
		roleCreate    *payloads.RoleCreate
		validatorErr  error
		apiError      errors.ApiError
	)

	BeforeEach(func() {
		roleCreate = new(payloads.RoleCreate)
		createPayload = payloads.RoleCreate{
			Type: "space_manager",
			Relationships: payloads.RoleRelationships{
				User: &payloads.UserRelationship{
					Data: payloads.UserRelationshipData{
						Username: "cf-service-account",
					},
				},
				Space: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "cf-space-guid",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), roleCreate)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(roleCreate).To(PointTo(Equal(createPayload)))
	})

	When("the user name is missing", func() {
		BeforeEach(func() {
			createPayload.Relationships.User = nil
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("User is a required field"))
		})
	})

	When("the user name and GUID are missing", func() {
		BeforeEach(func() {
			createPayload.Relationships.User = &payloads.UserRelationship{
				Data: payloads.UserRelationshipData{
					Username: "",
					GUID:     "",
				},
			}
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(SatisfyAll(
				ContainSubstring("Field validation for 'GUID' failed"),
				ContainSubstring("Field validation for 'Username' failed"),
			))
		})
	})

	When("the type is missing", func() {
		BeforeEach(func() {
			createPayload.Type = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("Type is a required field"))
		})
	})

	When("a space role is missing a space relationship", func() {
		BeforeEach(func() {
			createPayload.Relationships.Space = nil
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.space is a required field"))
		})
	})

	When("an org role is missing an org relationship", func() {
		BeforeEach(func() {
			createPayload.Type = "organization_manager"
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.organization is a required field"))
		})
	})

	When("both org and space relationships are set", func() {
		BeforeEach(func() {
			createPayload.Relationships.Organization = &payloads.Relationship{
				Data: &payloads.RelationshipData{
					GUID: "my-org",
				},
			}
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("Cannot pass both 'organization' and 'space' in a create role request"))
		})
	})

	Context("ToMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := roleCreate.ToMessage()
			Expect(msg.Type).To(Equal("space_manager"))
			Expect(msg.Space).To(Equal("cf-space-guid"))
			Expect(msg.User).To(Equal("cf-service-account"))
			Expect(msg.Kind).To(Equal(rbacv1.UserKind))
		})
	})

	When("the service account name is provided", func() {
		BeforeEach(func() {
			createPayload.Relationships.User.Data.Username = "system:serviceaccount:cf-space-guid:cf-service-account"
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(roleCreate).To(PointTo(Equal(createPayload)))
		})

		Context("ToMessage()", func() {
			It("converts to repo message correctly", func() {
				msg := roleCreate.ToMessage()
				Expect(msg.Type).To(Equal("space_manager"))
				Expect(msg.Space).To(Equal("cf-space-guid"))
				Expect(msg.User).To(Equal("cf-service-account"))
				Expect(msg.Kind).To(Equal(rbacv1.ServiceAccountKind))
				Expect(msg.ServiceAccountNamespace).To(Equal("cf-space-guid"))
			})
		})
	})
})

var _ = DescribeTable("Role org / space combination validation",
	func(role, orgOrSpace string, succeeds bool, errMsg string) {
		createRoleRequestBody := fmt.Sprintf(`{
				"type": "%s",
				"relationships": {
					"user": {
						"data": {
							"username": "my-user"
						}
					},
					"%s": {
						"data": {
							"guid": "some-guid"
						}
					}
				}
			}`, role, orgOrSpace)

		req, err := http.NewRequest("", "", bytes.NewReader([]byte(createRoleRequestBody)))
		Expect(err).NotTo(HaveOccurred())

		var roleCreate payloads.RoleCreate
		err = validator.DecodeAndValidateJSONPayload(req, &roleCreate)

		if succeeds {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
			apiError, ok := err.(errors.ApiError)
			Expect(ok).To(BeTrue(), "didn't get an errors.ApiError")
			Expect(apiError.Detail()).To(ContainSubstring(errMsg))
		}
	},

	Entry("org auditor w org", string(handlers.RoleOrganizationAuditor), "organization", true, ""),
	Entry("org auditor w space", string(handlers.RoleOrganizationAuditor), "space", false, "relationships.organization is a required field"),
	Entry("org billing manager w org", string(handlers.RoleOrganizationBillingManager), "organization", true, ""),
	Entry("org billing manager w space", string(handlers.RoleOrganizationBillingManager), "space", false, "relationships.organization is a required field"),
	Entry("org manager w org", string(handlers.RoleOrganizationManager), "organization", true, ""),
	Entry("org manager w space", string(handlers.RoleOrganizationManager), "space", false, "relationships.organization is a required field"),
	Entry("org user w org", string(handlers.RoleOrganizationUser), "organization", true, ""),
	Entry("org user w space", string(handlers.RoleOrganizationUser), "space", false, "relationships.organization is a required field"),

	Entry("space auditor w org", string(handlers.RoleSpaceAuditor), "organization", false, "relationships.space is a required field"),
	Entry("space auditor w space", string(handlers.RoleSpaceAuditor), "space", true, ""),
	Entry("space developer w org", string(handlers.RoleSpaceDeveloper), "organization", false, "relationships.space is a required field"),
	Entry("space developer w space", string(handlers.RoleSpaceDeveloper), "space", true, ""),
	Entry("space manager w org", string(handlers.RoleSpaceManager), "organization", false, "relationships.space is a required field"),
	Entry("space manager w space", string(handlers.RoleSpaceManager), "space", true, ""),
	Entry("space supporter w org", string(handlers.RoleSpaceSupporter), "organization", false, "relationships.space is a required field"),
	Entry("space supporter w space", string(handlers.RoleSpaceSupporter), "space", true, ""),

	Entry("invalid role name", "does-not-exist", "organization", false, "does-not-exist is not a valid role"),
)
