package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	rolesBase = "/v3/roles"
)

var _ = Describe("Role", func() {
	var (
		apiHandler *handlers.Role
		roleRepo   *fake.CFRoleRepository
		now        string
	)

	BeforeEach(func() {
		now = "2021-09-17T15:23:10Z"

		roleRepo = new(fake.CFRoleRepository)
		decoderValidator, err := handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler = handlers.NewRole(*serverURL, roleRepo, decoderValidator)
		routerBuilder.LoadRoutes(apiHandler)
	})

	DescribeTable("Role org / space combination validation",
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

			req, err := http.NewRequestWithContext(ctx, "POST", rolesBase, strings.NewReader(createRoleRequestBody))
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)

			if succeeds {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			} else {
				expectUnprocessableEntityError(errMsg)
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

	Describe("Create Role", func() {
		var createRoleRequestBody string

		BeforeEach(func() {
			roleRepo.CreateRoleStub = func(_ context.Context, _ authorization.Info, message repositories.CreateRoleMessage) (repositories.RoleRecord, error) {
				roleRecord := repositories.RoleRecord{
					GUID:      "t-h-e-r-o-l-e",
					CreatedAt: now,
					UpdatedAt: now,
					Type:      message.Type,
					Space:     message.Space,
					Org:       message.Org,
					User:      message.User,
					Kind:      message.Kind,
				}

				return roleRecord, nil
			}

			createRoleRequestBody = `{
				"type": "space_developer",
				"relationships": {
					"user": {
						"data": {
							"username": "my-user"
						}
					},
					"space": {
						"data": {
							"guid": "my-space"
						}
					}
				}
			}`
		})

		makePostRequest := func() {
			req, err := http.NewRequestWithContext(ctx, "POST", rolesBase, strings.NewReader(createRoleRequestBody))
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		}

		JustBeforeEach(func() {
			makePostRequest()
		})

		It("returns 201 with appropriate success JSON", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
				"guid": "t-h-e-r-o-l-e",
				"created_at": "2021-09-17T15:23:10Z",
				"updated_at": "2021-09-17T15:23:10Z",
				"type": "space_developer",
				"relationships": {
					"user": {
						"data":{
							"guid": "my-user"
						}
					},
					"space": {
						"data":{
							"guid": "my-space"
						}
					},
					"organization": {
						"data":null
					}
				},
				"links": {
					"self": {
						"href": "%[1]s/v3/roles/t-h-e-r-o-l-e"
					},
					"user": {
						"href": "%[1]s/v3/users/my-user"
					},
					"space": {
						"href": "%[1]s/v3/spaces/my-space"
					}
				}
			}`, defaultServerURL))))
		})

		It("invokes the role repo create function with expected parameters", func() {
			Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
			_, actualAuthInfo, roleRecord := roleRepo.CreateRoleArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(roleRecord.Type).To(Equal("space_developer"))
			Expect(roleRecord.Space).To(Equal("my-space"))
			Expect(roleRecord.User).To(Equal("my-user"))
			Expect(roleRecord.Kind).To(Equal(rbacv1.UserKind))
		})

		When("username is passed in the guid field", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "space_developer",
					"relationships": {
						"user": {
							"data": {
								"guid": "my-user"
							}
						},
						"space": {
							"data": {
								"guid": "my-space"
							}
						}
					}
				}`
			})

			It("still works as guid and username are equivalent here", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
					"guid": "t-h-e-r-o-l-e",
					"created_at": "2021-09-17T15:23:10Z",
					"updated_at": "2021-09-17T15:23:10Z",
					"type": "space_developer",
					"relationships": {
						"user": {
							"data":{
								"guid": "my-user"
							}
						},
						"space": {
							"data":{
								"guid": "my-space"
							}
						},
						"organization": {
							"data":null
						}
					},
					"links": {
						"self": {
							"href": "%[1]s/v3/roles/t-h-e-r-o-l-e"
						},
						"user": {
							"href": "%[1]s/v3/users/my-user"
						},
						"space": {
							"href": "%[1]s/v3/spaces/my-space"
						}
					}
				}`, defaultServerURL))))
			})
		})

		When("the role is an organisation role", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "organization_manager",
					"relationships": {
						"user": {
							"data": {
								"guid": "my-user"
							}
						},
						"organization": {
							"data": {
								"guid": "my-org"
							}
						}
					}
				}`
			})

			It("returns 201 with appropriate success JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
					"guid": "t-h-e-r-o-l-e",
					"created_at": "2021-09-17T15:23:10Z",
					"updated_at": "2021-09-17T15:23:10Z",
					"type": "organization_manager",
					"relationships": {
						"user": {
							"data":{
								"guid": "my-user"
							}
						},
						"space": {
							"data": null
						},
						"organization": {
							"data": {
								"guid": "my-org"
							}
						}
					},
					"links": {
						"self": {
							"href": "%[1]s/v3/roles/t-h-e-r-o-l-e"
						},
						"user": {
							"href": "%[1]s/v3/users/my-user"
						},
						"organization": {
							"href": "%[1]s/v3/organizations/my-org"
						}
					}
				}`, defaultServerURL))))
			})

			It("invokes the role repo create function with expected parameters", func() {
				Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
				_, _, roleRecord := roleRepo.CreateRoleArgsForCall(0)
				Expect(roleRecord.Type).To(Equal("organization_manager"))
				Expect(roleRecord.Org).To(Equal("my-org"))
				Expect(roleRecord.User).To(Equal("my-user"))
			})
		})

		When("the kind is a service account", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "organization_manager",
					"relationships": {
						"kubernetesServiceAccount": {
							"data": {
								"guid": "my-user"
							}
						},
						"organization": {
							"data": {
								"guid": "my-org"
							}
						}
					}
				}`
			})

			It("creates a service account role binding", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(roleRepo.CreateRoleCallCount()).To(Equal(1))
				_, _, roleRecord := roleRepo.CreateRoleArgsForCall(0)
				Expect(roleRecord.Type).To(Equal("organization_manager"))
				Expect(roleRecord.Org).To(Equal("my-org"))
				Expect(roleRecord.User).To(Equal("my-user"))
				Expect(roleRecord.Kind).To(Equal(rbacv1.ServiceAccountKind))
			})
		})

		When("the role does not contain a user or service account", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "organization_manager",
					"relationships": {
						"organization": {
							"data": {
								"guid": "my-org"
							}
						}
					}
				}`
			})

			It("returns an error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					ContainSubstring("Field validation for 'User' failed on the 'required_without' tag"),
					ContainSubstring("Field validation for 'KubernetesServiceAccount' failed on the 'required_without' tag"),
				)))
			})
		})

		When("the role does not contain a space or organisation relationship", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "organization_manager",
					"relationships": {
						"user": {
							"data": {
								"guid": "my-user"
							}
						}
					}
				}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("relationships.organization is a required field")
			})
		})

		When("the role does contains both a space and organization relationship", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "organization_manager",
					"relationships": {
						"user": {
							"data": {
								"guid": "my-user"
							}
						},
						"space": {
							"data": {
								"guid": "my-space"
							}
						},
						"organization": {
							"data": {
								"guid": "my-org"
							}
						}
					}
				}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Cannot pass both 'organization' and 'space' in a create role request")
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

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				createRoleRequestBody = "{"
			})

			It("returns a status 400 with appropriate error JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
					"errors": [
						{
							"title": "CF-MessageParseError",
							"detail": "Request invalid due to parse error: invalid request body",
							"code": 1001
						}
					]
				}`)))
			})
		})

		When("the request body has an unknown field", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{"who-am-i":"dunno"}`
			})

			It("returns a status 422 with appropriate error JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
					"errors": [
						{
							"title": "CF-UnprocessableEntity",
							"detail": "invalid request body: json: unknown field \"who-am-i\"",
							"code": 10008
						}
					]
				}`)))
			})
		})

		When("the request body is invalid with missing required type field", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"relationships": {
						"user": {
							"data": {
								"guid": "my-user"
							}
						},
						"space": {
							"data": {
								"guid": "my-space"
							}
						}
					}
				}`
			})

			It("returns a status 422 with appropriate error message json", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
					"errors": [
						{
							"title": "CF-UnprocessableEntity",
							"detail": "Type is a required field",
							"code": 10008
						}
					]
				}`)))
			})
		})

		When("the request body has neither user name nor guid", func() {
			BeforeEach(func() {
				createRoleRequestBody = `{
					"type": "organization_manager",
					"relationships": {
						"user": {
							"data": {}
						},
						"space": {
							"data": {
								"guid": "my-space"
							}
						}
					}
				}`
			})

			It("returns a status 422 with appropriate error message json", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					ContainSubstring("Field validation for 'GUID' failed"),
					ContainSubstring("Field validation for 'Username' failed"),
				)))
			})
		})
	})

	Describe("List roles", func() {
		var query string

		BeforeEach(func() {
			query = ""
			roleRepo.ListRolesReturns([]repositories.RoleRecord{
				{
					GUID:      "org-role",
					CreatedAt: now,
					UpdatedAt: now,
					Type:      "organization_manager",
					Space:     "",
					Org:       "the-org",
					User:      "org-user",
				},
				{
					GUID:      "space-role",
					CreatedAt: now,
					UpdatedAt: now,
					Type:      "space_developer",
					Space:     "the-space",
					Org:       "",
					User:      "space-user",
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", rolesBase+query, nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("calls the roles repo correctly", func() {
			Expect(roleRepo.ListRolesCallCount()).To(Equal(1))
			_, actualAuthInfo := roleRepo.ListRolesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
		})

		It("lists roles", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
				"pagination": {
					"total_results": 2,
					"total_pages": 1,
					"first": {
						"href": "%[1]s/v3/roles"
					},
					"last": {
						"href": "%[1]s/v3/roles"
					},
					"next": null,
					"previous": null
				},
				"resources": [
					{
						"guid": "org-role",
						"created_at": "2021-09-17T15:23:10Z",
						"updated_at": "2021-09-17T15:23:10Z",
						"type": "organization_manager",
						"relationships": {
							"user": {
								"data": {
									"guid": "org-user"
								}
							},
							"organization": {
								"data": {
									"guid": "the-org"
								}
							},
							"space": {
								"data": null
							}
						},
						"links": {
							"self": {
								"href": "%[1]s/v3/roles/org-role"
							},
							"user": {
								"href": "%[1]s/v3/users/org-user"
							},
							"organization": {
								"href": "%[1]s/v3/organizations/the-org"
							}
						}
					},
					{
						"guid": "space-role",
						"created_at": "2021-09-17T15:23:10Z",
						"updated_at": "2021-09-17T15:23:10Z",
						"type": "space_developer",
						"relationships": {
							"user": {
								"data": {
									"guid": "space-user"
								}
							},
							"organization": {
								"data": null
							},
							"space": {
								"data": {
									"guid": "the-space"
								}
							}
						},
						"links": {
							"self": {
								"href": "%[1]s/v3/roles/space-role"
							},
							"user": {
								"href": "%[1]s/v3/users/space-user"
							},
							"space": {
								"href": "%[1]s/v3/spaces/the-space"
							}
						}
					}
				]
			}`, defaultServerURL))))
		})

		When("include is specified", func() {
			BeforeEach(func() {
				query = "?include=user"
			})

			It("does not fail but has no effect on the result", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					ContainSubstring("org-user"),
					ContainSubstring("the-org"),
					ContainSubstring("space-user"),
					ContainSubstring("the-space"),
				)))
			})
		})

		type res struct {
			GUID string `json:"guid"`
		}
		type resList struct {
			Resources []res `json:"resources"`
		}

		DescribeTable("filtering", func(filter string, expectedGUIDs ...string) {
			roleRepo.ListRolesReturns([]repositories.RoleRecord{
				{GUID: "1", Type: "a", Space: "space1", Org: "", User: "user1"},
				{GUID: "2", Type: "b", Space: "space2", Org: "", User: "user1"},
				{GUID: "3", Type: "c", Space: "", Org: "org1", User: "user1"},
				{GUID: "4", Type: "c", Space: "", Org: "org2", User: "user2"},
			}, nil)
			req, err := http.NewRequestWithContext(ctx, "GET", rolesBase+"?"+filter, nil)
			Expect(err).NotTo(HaveOccurred())
			rr = httptest.NewRecorder()
			routerBuilder.Build().ServeHTTP(rr, req)
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))

			var respList resList
			err = json.Unmarshal(rr.Body.Bytes(), &respList)
			Expect(err).NotTo(HaveOccurred())

			var expectedRes []res
			for _, guid := range expectedGUIDs {
				expectedRes = append(expectedRes, res{GUID: guid})
			}
			Expect(respList.Resources).To(ConsistOf(expectedRes))
		},
			Entry("no filter", "", "1", "2", "3", "4"),
			Entry("guids1", "guids=4", "4"),
			Entry("guids2", "guids=1,3", "1", "3"),
			Entry("types1", "types=a", "1"),
			Entry("types2", "types=b,c", "2", "3", "4"),
			Entry("space_guids1", "space_guids=space1", "1"),
			Entry("space_guids2", "space_guids=space1,space2", "1", "2"),
			Entry("organization_guids1", "organization_guids=org1", "3"),
			Entry("organization_guids2", "organization_guids=org1,org2", "3", "4"),
			Entry("user_guids1", "user_guids=user1", "1", "2", "3"),
			Entry("user_guids2", "user_guids=user1,user2", "1", "2", "3", "4"),
		)

		DescribeTable("ordering", func(order string, expectedGUIDs ...string) {
			roleRepo.ListRolesReturns([]repositories.RoleRecord{
				{GUID: "1", CreatedAt: "2022-01-23T17:08:22", UpdatedAt: "2022-01-22T17:09:00"},
				{GUID: "2", CreatedAt: "2022-01-24T17:08:22", UpdatedAt: "2022-01-21T17:09:00"},
				{GUID: "3", CreatedAt: "2022-01-22T17:08:22", UpdatedAt: "2022-01-24T17:09:00"},
				{GUID: "4", CreatedAt: "2022-01-21T17:08:22", UpdatedAt: "2022-01-23T17:09:00"},
			}, nil)
			req, err := http.NewRequestWithContext(ctx, "GET", rolesBase+"?order_by="+order, nil)
			Expect(err).NotTo(HaveOccurred())
			rr = httptest.NewRecorder()
			routerBuilder.Build().ServeHTTP(rr, req)
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))

			var respList resList
			err = json.Unmarshal(rr.Body.Bytes(), &respList)
			Expect(err).NotTo(HaveOccurred())

			var expectedRes []res
			for _, guid := range expectedGUIDs {
				expectedRes = append(expectedRes, res{GUID: guid})
			}
			Expect(respList.Resources).To(Equal(expectedRes))
		},
			Entry("created_at ASC", "created_at", "4", "3", "1", "2"),
			Entry("created_at DESC", "-created_at", "2", "1", "3", "4"),
			Entry("updated_at ASC", "updated_at", "2", "1", "4", "3"),
			Entry("updated_at DESC", "-updated_at", "3", "4", "1", "2"),
		)

		When("order_by is not a valid field", func() {
			BeforeEach(func() {
				query = "?order_by=not_valid"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Order by can only be: 'created_at', 'updated_at'")
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
})
