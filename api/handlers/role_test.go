package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
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
			roleRepo.ListRolesReturns([]repositories.RoleRecord{
				{GUID: "role-1"},
				{GUID: "role-2"},
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
			_, actualAuthInfo := roleRepo.ListRolesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/roles?foo=bar"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "role-1"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/roles/role-1"),
				MatchJSONPath("$.resources[1].guid", "role-2"),
			)))
		})

		Describe("filtering and ordering", func() {
			BeforeEach(func() {
				roleRepo.ListRolesReturns([]repositories.RoleRecord{
					{GUID: "1", CreatedAt: "2022-01-23T17:08:22", UpdatedAt: "2022-01-22T17:09:00", Type: "a", Space: "space1", Org: "", User: "user1"},
					{GUID: "2", CreatedAt: "2022-01-24T17:08:22", UpdatedAt: "2022-01-21T17:09:00", Type: "b", Space: "space2", Org: "", User: "user1"},
					{GUID: "3", CreatedAt: "2022-01-22T17:08:22", UpdatedAt: "2022-01-24T17:09:00", Type: "c", Space: "", Org: "org1", User: "user1"},
					{GUID: "4", CreatedAt: "2022-01-21T17:08:22", UpdatedAt: "2022-01-23T17:09:00", Type: "c", Space: "", Org: "org2", User: "user2"},
				}, nil)
			})

			DescribeTable("filtering", func(filter payloads.RoleList, expectedGUIDs ...any) {
				req, err := http.NewRequestWithContext(ctx, "GET", rolesBase+"?foo=bar", nil)
				Expect(err).NotTo(HaveOccurred())

				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&filter)
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.resources[*].guid", expectedGUIDs)))
			},
				Entry("no filter", payloads.RoleList{}, "1", "2", "3", "4"),
				Entry("guids1", payloads.RoleList{GUIDs: map[string]bool{"4": true}}, "4"),
				Entry("guids2", payloads.RoleList{GUIDs: map[string]bool{"1": true, "3": true}}, "1", "3"),
				Entry("types1", payloads.RoleList{Types: map[string]bool{"a": true}}, "1"),
				Entry("types2", payloads.RoleList{Types: map[string]bool{"b": true, "c": true}}, "2", "3", "4"),
				Entry("space_guids1", payloads.RoleList{SpaceGUIDs: map[string]bool{"space1": true}}, "1"),
				Entry("space_guids2", payloads.RoleList{SpaceGUIDs: map[string]bool{"space1": true, "space2": true}}, "1", "2"),
				Entry("organization_guids1", payloads.RoleList{OrgGUIDs: map[string]bool{"org1": true}}, "3"),
				Entry("organization_guids2", payloads.RoleList{OrgGUIDs: map[string]bool{"org1": true, "org2": true}}, "3", "4"),
				Entry("user_guids1", payloads.RoleList{UserGUIDs: map[string]bool{"user1": true}}, "1", "2", "3"),
				Entry("user_guids2", payloads.RoleList{UserGUIDs: map[string]bool{"user1": true, "user2": true}}, "1", "2", "3", "4"),
			)

			DescribeTable("ordering", func(order string, expectedGUIDs ...any) {
				req, err := http.NewRequestWithContext(ctx, "GET", rolesBase+"?order_by=not-used-in-test", nil)
				Expect(err).NotTo(HaveOccurred())

				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.RoleList{OrderBy: order})
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.resources[*].guid", expectedGUIDs)))
			},
				Entry("created_at ASC", "created_at", "4", "3", "1", "2"),
				Entry("created_at DESC", "-created_at", "2", "1", "3", "4"),
				Entry("updated_at ASC", "updated_at", "2", "1", "4", "3"),
				Entry("updated_at DESC", "-updated_at", "3", "4", "1", "2"),
			)
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
				roleRepo.ListRolesReturns(nil, errors.New("boom"))
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
