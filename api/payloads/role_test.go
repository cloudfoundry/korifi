package payloads_test

import (
	"bytes"
	"fmt"
	"net/http"

	rbacv1 "k8s.io/api/rbac/v1"

	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tests/helpers"

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
				User: payloads.UserRelationship{
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
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), roleCreate)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(roleCreate).To(PointTo(Equal(createPayload)))
	})

	When("the user name and GUID are missing", func() {
		BeforeEach(func() {
			createPayload.Relationships.User.Data.GUID = ""
			createPayload.Relationships.User.Data.Username = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("user cannot be blank"))
		})
	})

	When("the type is missing", func() {
		BeforeEach(func() {
			createPayload.Type = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("type cannot be blank"))
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
			Expect(apiError.Detail()).To(ContainSubstring("cannot pass both 'organization' and 'space' in a create role request"))
		})
	})

	Context("ToMessage()", func() {
		var msg repositories.CreateRoleMessage

		JustBeforeEach(func() {
			msg = roleCreate.ToMessage()
		})

		It("converts to repo message correctly", func() {
			Expect(msg.Type).To(Equal("space_manager"))
			Expect(msg.Space).To(Equal("cf-space-guid"))
			Expect(msg.User).To(Equal("cf-service-account"))
			Expect(msg.Kind).To(Equal(rbacv1.UserKind))
		})

		When("user origin is specified", func() {
			BeforeEach(func() {
				createPayload.Relationships.User.Data.Origin = "my-origin"
			})

			It("uses the origin in the message user", func() {
				Expect(msg.User).To(Equal("my-origin:cf-service-account"))
			})
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

	Entry("org auditor w org", payloads.RoleOrganizationAuditor, "organization", true, ""),
	Entry("org auditor w space", payloads.RoleOrganizationAuditor, "space", false, "relationships.organization is required"),
	Entry("org billing manager w org", payloads.RoleOrganizationBillingManager, "organization", true, ""),
	Entry("org billing manager w space", payloads.RoleOrganizationBillingManager, "space", false, "relationships.organization is required"),
	Entry("org manager w org", payloads.RoleOrganizationManager, "organization", true, ""),
	Entry("org manager w space", payloads.RoleOrganizationManager, "space", false, "relationships.organization is required"),
	Entry("org user w org", payloads.RoleOrganizationUser, "organization", true, ""),
	Entry("org user w space", payloads.RoleOrganizationUser, "space", false, "relationships.organization is required"),

	Entry("space auditor w org", payloads.RoleSpaceAuditor, "organization", false, "relationships.space is required"),
	Entry("space auditor w space", payloads.RoleSpaceAuditor, "space", true, ""),
	Entry("space developer w org", payloads.RoleSpaceDeveloper, "organization", false, "relationships.space is required"),
	Entry("space developer w space", payloads.RoleSpaceDeveloper, "space", true, ""),
	Entry("space manager w org", payloads.RoleSpaceManager, "organization", false, "relationships.space is required"),
	Entry("space manager w space", payloads.RoleSpaceManager, "space", true, ""),
	Entry("space supporter w org", payloads.RoleSpaceSupporter, "organization", false, "relationships.space is required"),
	Entry("space supporter w space", payloads.RoleSpaceSupporter, "space", true, ""),

	Entry("invalid role name", "does-not-exist", "organization", false, "type value must be one of"),
)

var _ = Describe("RoleList", func() {
	DescribeTable("valid query",
		func(query string, expectedRoleListQueryParameters payloads.RoleList) {
			actualRoleListQueryParameters, decodeErr := decodeQuery[payloads.RoleList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualRoleListQueryParameters).To(Equal(expectedRoleListQueryParameters))
		},

		Entry("guids", "guids=g1,g2", payloads.RoleList{GUIDs: map[string]bool{"g1": true, "g2": true}}),
		Entry("types", "types=g1,g2", payloads.RoleList{Types: map[string]bool{"g1": true, "g2": true}}),
		Entry("space_guids", "space_guids=g1,g2", payloads.RoleList{SpaceGUIDs: map[string]bool{"g1": true, "g2": true}}),
		Entry("organization_guids", "organization_guids=g1,g2", payloads.RoleList{OrgGUIDs: map[string]bool{"g1": true, "g2": true}}),
		Entry("user_guids", "user_guids=g1,g2", payloads.RoleList{UserGUIDs: map[string]bool{"g1": true, "g2": true}}),
		Entry("order_by1", "order_by=created_at", payloads.RoleList{OrderBy: "created_at"}),
		Entry("order_by2", "order_by=-created_at", payloads.RoleList{OrderBy: "-created_at"}),
		Entry("order_by3", "order_by=updated_at", payloads.RoleList{OrderBy: "updated_at"}),
		Entry("order_by4", "order_by=-updated_at", payloads.RoleList{OrderBy: "-updated_at"}),
		Entry("page=3", "page=3", payloads.RoleList{Pagination: payloads.Pagination{Page: "3"}}),
		Entry("include", "include=foo", payloads.RoleList{}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.RoleList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid order_by", "order_by=foo", "value must be one of"),
		Entry("page=foo", "page=foo", "value must be an integer"),
	)

	Describe("ToMessage", func() {
		var (
			roleList payloads.RoleList
			message  repositories.ListRolesMessage
		)

		BeforeEach(func() {
			roleList = payloads.RoleList{
				GUIDs:      helpers.Set("g1", "g2"),
				Types:      helpers.Set("space_manager", "space_auditor"),
				SpaceGUIDs: helpers.Set("space1", "space2"),
				OrgGUIDs:   helpers.Set("org1", "org2"),
				UserGUIDs:  helpers.Set("user1", "user2"),
				OrderBy:    "created_at",
				Pagination: payloads.Pagination{
					PerPage: "10",
					Page:    "4",
				},
			}
		})

		JustBeforeEach(func() {
			message = roleList.ToMessage()
		})

		It("translates to repository message", func() {
			Expect(message).To(Equal(repositories.ListRolesMessage{
				GUIDs:      helpers.Set("g1", "g2"),
				Types:      helpers.Set("space_manager", "space_auditor"),
				SpaceGUIDs: helpers.Set("space1", "space2"),
				OrgGUIDs:   helpers.Set("org1", "org2"),
				UserGUIDs:  helpers.Set("user1", "user2"),
				OrderBy:    "created_at",
				Pagination: repositories.Pagination{
					PerPage: 10,
					Page:    4,
				},
			}))
		})
	})
})
