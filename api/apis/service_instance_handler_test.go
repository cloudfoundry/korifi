package apis_test

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceInstanceHandler", func() {
	const (
		testServiceInstanceHandlerLoggerName = "TestServiceInstanceHandler"
		serviceInstanceGUID                  = "test-service-instance-guid"
		serviceInstanceSpaceGUID             = "test-space-guid"
		serviceInstanceTypeUserProvided      = "user-provided"
	)

	var (
		req                 *http.Request
		serviceInstanceRepo *fake.CFServiceInstanceRepository
		appRepo             *fake.CFAppRepository
	)

	BeforeEach(func() {
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		appRepo = new(fake.CFAppRepository)
		serviceInstanceHandler := NewServiceInstanceHandler(
			logf.Log.WithName(testServiceInstanceHandlerLoggerName),
			*serverURL,
			serviceInstanceRepo,
			appRepo,
		)
		serviceInstanceHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the POST /v3/service_instances endpoint", func() {
		makePostRequest := func(body string) {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/service_instances", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
		}

		const (
			serviceInstanceName = "my-upsi"
			createdAt           = "1906-04-18T13:12:00Z"
			updatedAt           = "1906-04-18T13:12:00Z"
			validBody           = `{
				"name": "` + serviceInstanceName + `",
				"tags": ["foo", "bar"],
				"relationships": {
					"space": {
						"data": {
							"guid": "` + serviceInstanceSpaceGUID + `"
						}
					}
				},
				"type": "` + serviceInstanceTypeUserProvided + `"
			}`
		)

		When("on the happy path", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{
					Name:       serviceInstanceName,
					GUID:       serviceInstanceGUID,
					SpaceGUID:  serviceInstanceSpaceGUID,
					SecretName: serviceInstanceGUID,
					Tags:       []string{"foo", "bar"},
					Type:       serviceInstanceTypeUserProvided,
					CreatedAt:  createdAt,
					UpdatedAt:  updatedAt,
				}, nil)

				makePostRequest(validBody)
			})

			It("returns status 201 CREATED", func() {
				Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
			})

			It("creates a CFServiceInstance", func() {
				Expect(serviceInstanceRepo.CreateServiceInstanceCallCount()).To(Equal(1))
				_, actualAuthInfo, actualCreate := serviceInstanceRepo.CreateServiceInstanceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualCreate).To(Equal(repositories.CreateServiceInstanceMessage{
					Name:      serviceInstanceName,
					SpaceGUID: serviceInstanceSpaceGUID,
					Type:      serviceInstanceTypeUserProvided,
					Tags:      []string{"foo", "bar"},
				}))
			})

			It("returns the ServiceInstance in the response", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
				  "created_at": "%[4]s",
				  "guid": "%[2]s",
				  "last_operation": {
					"created_at": "%[4]s",
					"description": "Operation succeeded",
					"state": "succeeded",
					"type": "create",
					"updated_at": "%[5]s"
				  },
				  "links": {
					"credentials": {
					  "href": "%[1]s/v3/service_instances/%[2]s/credentials"
					},
					"self": {
					  "href": "%[1]s/v3/service_instances/%[2]s"
					},
					"service_credential_bindings": {
					  "href": "%[1]s/v3/service_credential_bindings?service_instance_guids=%[2]s"
					},
					"service_route_bindings": {
					  "href": "%[1]s/v3/service_route_bindings?service_instance_guids=%[2]s"
					},
					"space": {
					  "href": "%[1]s/v3/spaces/%[3]s"
					}
				  },
				  "metadata": {
					"annotations": {},
					"labels": {}
				  },
				  "name": "%[6]s",
				  "relationships": {
					"space": {
					  "data": {
						"guid": "%[3]s"
					  }
					}
				  },
				  "route_service_url": null,
				  "syslog_drain_url": null,
				  "tags": ["foo", "bar"],
				  "type": "user-provided",
				  "updated_at": "%[5]s"
				}`, defaultServerURL, serviceInstanceGUID, serviceInstanceSpaceGUID, createdAt, updatedAt, serviceInstanceName)), "Response body matches response:")
			})
		})

		When("the request body is not valid", func() {
			BeforeEach(func() {
				makePostRequest(`{"description" : "Invalid Request"}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "description"`)
			})
		})

		When("the request body has route_service_url set", func() {
			BeforeEach(func() {
				makePostRequest(`{"route_service_url" : "Invalid Request"}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "route_service_url"`)
			})
		})

		When("the request body has syslog_drain_url set", func() {
			BeforeEach(func() {
				makePostRequest(`{"syslog_drain_url" : "Invalid Request"}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "syslog_drain_url"`)
			})
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
				makePostRequest(`{
				"relationships": {
					"space": {
						"data": {
							"guid": "` + serviceInstanceSpaceGUID + `"
						}
					}
				},
				"type": "` + serviceInstanceTypeUserProvided + `"
			}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Name is a required field")
			})
		})

		When("the request body is invalid with invalid name", func() {
			BeforeEach(func() {
				makePostRequest(`{
				"name": 12345,
				"relationships": {
					"space": {
						"data": {
							"guid": "` + serviceInstanceSpaceGUID + `"
						}
					}
				},
				"type": "` + serviceInstanceTypeUserProvided + `"
			}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Name must be a string")
			})
		})

		When("the request body is invalid with missing required type field", func() {
			BeforeEach(func() {
				makePostRequest(`{
				"name": "` + serviceInstanceName + `",
				"relationships": {
					"space": {
						"data": {
							"guid": "` + serviceInstanceSpaceGUID + `"
						}
					}
				}
			}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Type is a required field")
			})
		})

		When("the request body is invalid with invalid type", func() {
			BeforeEach(func() {
				makePostRequest(`{
				"name": "` + serviceInstanceName + `",
				"relationships": {
					"space": {
						"data": {
							"guid": "` + serviceInstanceSpaceGUID + `"
						}
					}
				},
				"type": "managed"
			}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Type must be one of [user-provided]")
			})
		})

		When("the request body is invalid with missing relationship field", func() {
			BeforeEach(func() {
				makePostRequest(`{
				"name": "` + serviceInstanceName + `",
				"type": "` + serviceInstanceTypeUserProvided + `"
			}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Data is a required field")
			})
		})

		When("the request body is invalid with Tags that combine to exceed length 2048", func() {
			BeforeEach(func() {
				makePostRequest(`{
				"name": "` + serviceInstanceName + `",
				"tags": ["` + randomString(2048) + `"],
				"relationships": {
					"space": {
						"data": {
							"guid": "` + serviceInstanceSpaceGUID + `"
						}
					}
				},
				"type": "` + serviceInstanceTypeUserProvided + `"
			}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Key: 'ServiceInstanceCreate.Tags' Error:Field validation for 'Tags' failed on the 'serviceinstancetaglength' tag")
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				appRepo.GetNamespaceReturns(
					repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")},
				)

				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("the get namespace returns an unknown error", func() {
			BeforeEach(func() {
				appRepo.GetNamespaceReturns(
					repositories.SpaceRecord{},
					errors.New("unknown"),
				)

				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{}, authorization.InvalidAuthError{})
				makePostRequest(validBody)
			})

			It("returns Invalid Auth error", func() {
				expectInvalidAuthError()
			})
		})

		When("authentication is not provided", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{}, authorization.NotAuthenticatedError{})
				makePostRequest(validBody)
			})

			It("returns a NotAuthenticated error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("user is not allowed to create a service instance", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{}, repositories.NewForbiddenError(errors.New("nope")))
				makePostRequest(validBody)
			})

			It("returns an unauthorised error", func() {
				expectUnauthorizedError()
			})
		})

		When("providing the service instance repository fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("space-repo-provisioning-failed"))
				makePostRequest(validBody)
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})
	})
})

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}
