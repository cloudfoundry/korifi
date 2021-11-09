package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	rolesBase = "/v3/roles"
)

var _ = Describe("RoleHandler", func() {
	var (
		ctx         context.Context
		roleHandler *apis.RoleHandler
		roleRepo    *fake.CFRoleRepository
		now         time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z

		roleRepo = new(fake.CFRoleRepository)

		roleHandler = apis.NewRoleHandler(*serverURL, roleRepo)
		roleHandler.RegisterRoutes(router)
	})

	Describe("Create Role", func() {
		var createRoleRequestBody string

		BeforeEach(func() {
			roleRepo.CreateSpaceRoleStub = func(_ context.Context, role repositories.RoleRecord) (repositories.RoleRecord, error) {
				role.GUID = "t-h-e-r-o-l-e"
				role.CreatedAt = now
				role.UpdatedAt = now

				return role, nil
			}

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

		makePostRequest := func() {
			req, err := http.NewRequestWithContext(ctx, "POST", rolesBase, strings.NewReader(createRoleRequestBody))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
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
						"space": {
							"href": "%[1]s/v3/spaces/my-space"
						}
					}
				}`, defaultServerURL))))
		})

		It("invokes the role repo create function with expected parameters", func() {
			Expect(roleRepo.CreateSpaceRoleCallCount()).To(Equal(1))
			_, roleRecord := roleRepo.CreateSpaceRoleArgsForCall(0)
			Expect(roleRecord.Type).To(Equal("space_developer"))
			Expect(roleRecord.Space).To(Equal("my-space"))
			Expect(roleRecord.User).To(Equal("my-user"))
		})

		When("the user has been already assigned to that role", func() {
			BeforeEach(func() {
				roleRepo.CreateSpaceRoleReturns(repositories.RoleRecord{}, repositories.ErrorDuplicateRoleBinding)
			})

			It("returns unprocessable entry error", func() {
				expectUnprocessableEntityError("User 'my-user' already has 'space_developer' role")
			})
		})

		When("the user does not have a role in the parent organization", func() {
			BeforeEach(func() {
				roleRepo.CreateSpaceRoleReturns(repositories.RoleRecord{}, repositories.ErrorMissingRoleBindingInParentOrg)
			})

			It("returns unprocessable entry error", func() {
				expectUnprocessableEntityError("Users cannot be assigned roles in a space if they do not have a role in that space's organization.")
			})
		})

		When("the org repo returns another error", func() {
			BeforeEach(func() {
				roleRepo.CreateSpaceRoleReturns(repositories.RoleRecord{}, errors.New("boom"))
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
	})
})
