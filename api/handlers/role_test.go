package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	rolesBase = "/v3/roles"
)

var _ = Describe("Role", func() {
	var (
		apiHandler       *handlers.Role
		roleRepo         *fake.CFRoleRepository
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		roleRepo = new(fake.CFRoleRepository)
		requestValidator = new(fake.RequestValidator)

		apiHandler = handlers.NewRole(*serverURL, roleRepo, requestValidator)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("Create Role", func() {
		var roleCreate *payloads.RoleCreate

		BeforeEach(func() {
			roleRepo.CreateRoleReturns(repositories.RoleRecord{GUID: "role-guid"}, nil)
			roleCreate = &payloads.RoleCreate{
				Type: "space_developer",
				Relationships: payloads.RoleRelationships{
					User: payloads.UserRelationship{
						Data: payloads.UserRelationshipData{
							Username: "my-user",
						},
					},
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "my-space",
						},
					},
				},
			}

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(roleCreate)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", rolesBase, strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("creates the role", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
			_, actualAuthInfo, roleMessage := roleRepo.CreateRoleArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(roleMessage.Type).To(Equal("space_developer"))
			Expect(roleMessage.Space).To(Equal("my-space"))
			Expect(roleMessage.User).To(Equal("my-user"))
			Expect(roleMessage.Kind).To(Equal(rbacv1.UserKind))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "role-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/roles/role-guid"),
			)))
		})

		When("username is passed in the guid field", func() {
			BeforeEach(func() {
				roleCreate.Relationships.User.Data.Username = ""
				roleCreate.Relationships.User.Data.GUID = "my-user"
			})

			It("still works as guid and username are equivalent here", func() {
				Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
				_, _, roleMessage := roleRepo.CreateRoleArgsForCall(0)
				Expect(roleMessage.User).To(Equal("my-user"))
			})
		})

		When("the role is an organisation role", func() {
			BeforeEach(func() {
				roleCreate.Type = "organization_manager"
				roleCreate.Relationships.Space = nil
				roleCreate.Relationships.Organization = &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "my-org",
					},
				}
			})

			It("invokes the role repo create function with expected parameters", func() {
				Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
				_, _, roleMessage := roleRepo.CreateRoleArgsForCall(0)
				Expect(roleMessage.Type).To(Equal("organization_manager"))
				Expect(roleMessage.Org).To(Equal("my-org"))
			})
		})

		When("the kind is a service account", func() {
			BeforeEach(func() {
				roleCreate.Relationships.User.Data.GUID = "system:serviceaccount:cf:my-user"
			})

			It("creates a service account role binding", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
				_, _, roleRecord := roleRepo.CreateRoleArgsForCall(0)
				Expect(roleRecord.User).To(Equal("my-user"))
				Expect(roleRecord.ServiceAccountNamespace).To(Equal("cf"))
				Expect(roleRecord.Kind).To(Equal(rbacv1.ServiceAccountKind))
			})
		})

		When("the payload validator returns an error", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("foo"), "some error"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("some error")
			})
		})

		When("the org repo returns another error", func() {
			BeforeEach(func() {
				roleRepo.CreateRoleReturns(repositories.RoleRecord{}, errors.New("boom"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("List roles", func() {
		BeforeEach(func() {
			roleRepo.ListRolesReturns(repositories.ListResult[repositories.RoleRecord]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 2,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     2,
				},
				Records: []repositories.RoleRecord{
					{GUID: "role-1"},
					{GUID: "role-2"},
				},
			}, nil)
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.RoleList{})
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", rolesBase+"?foo=bar", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("lists roles", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			req, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(req.URL.String()).To(HaveSuffix(rolesBase + "?foo=bar"))

			Expect(roleRepo.ListRolesCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := roleRepo.ListRolesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.ListRolesMessage{
				Pagination: repositories.Pagination{PerPage: 50, Page: 1},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "role-1"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/roles/role-1"),
				MatchJSONPath("$.resources[1].guid", "role-2"),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.RoleList{
					GUIDs:      "g1,g2",
					Types:      "space_manager,space_auditor",
					SpaceGUIDs: "space1,space2",
					OrgGUIDs:   "org1,org2",
					UserGUIDs:  "user1,user2",
					OrderBy:    "created_at",
					Pagination: payloads.Pagination{PerPage: "16", Page: "32"},
				})
			})

			It("passes them to the repository", func() {
				Expect(roleRepo.ListRolesCallCount()).To(Equal(1))
				_, _, message := roleRepo.ListRolesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListRolesMessage{
					GUIDs:      []string{"g1", "g2"},
					Types:      []string{"space_manager", "space_auditor"},
					SpaceGUIDs: []string{"space1", "space2"},
					OrgGUIDs:   []string{"org1", "org2"},
					UserGUIDs:  []string{"user1", "user2"},
					OrderBy:    "created_at",
					Pagination: repositories.Pagination{PerPage: 16, Page: 32},
				}))
			})
		})

		When("decoding the url values fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boom"))
			})

			It("returns an Unknown error", func() {
				expectUnknownError()
			})
		})

		When("calling the repository fails", func() {
			BeforeEach(func() {
				roleRepo.ListRolesReturns(repositories.ListResult[repositories.RoleRecord]{}, errors.New("boom"))
			})

			It("returns the error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("delete a role", func() {
		BeforeEach(func() {
			roleRepo.GetRoleReturns(repositories.RoleRecord{
				GUID:  "role-guid",
				Space: "my-space",
				Org:   "",
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "DELETE", rolesBase+"/role-guid", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("deletes the role", func() {
			Expect(roleRepo.GetRoleCallCount()).To(Equal(1))
			_, actualAuthInfo, actualRoleGuid := roleRepo.GetRoleArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualRoleGuid).To(Equal("role-guid"))

			Expect(roleRepo.DeleteRoleCallCount()).To(Equal(1))
			_, actualAuthInfo, roleDeleteMsg := roleRepo.DeleteRoleArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(roleDeleteMsg).To(Equal(repositories.DeleteRoleMessage{
				GUID:  "role-guid",
				Space: "my-space",
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", ContainSubstring("jobs/role.delete~role-guid")))
		})

		When("getting the role is forbidden", func() {
			BeforeEach(func() {
				roleRepo.GetRoleReturns(repositories.RoleRecord{}, apierrors.NewForbiddenError(nil, "Role"))
			})

			It("returns a not found error", func() {
				expectNotFoundError("Role")
			})
		})

		When("getting the role fails", func() {
			BeforeEach(func() {
				roleRepo.GetRoleReturns(repositories.RoleRecord{}, errors.New("get-role-err"))
			})

			It("returns the error", func() {
				expectUnknownError()
			})
		})

		When("deleting the role from the repo fails", func() {
			BeforeEach(func() {
				roleRepo.DeleteRoleReturns(errors.New("delete-role-err"))
			})

			It("returns the error", func() {
				expectUnknownError()
			})
		})
	})
})
