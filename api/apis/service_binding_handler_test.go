package apis_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceBindingHandler", func() {
	const (
		testServiceBindingHandlerLoggerName = "TestServiceBindingHandler"
		appGUID                             = "test-app-guid"
		serviceBindingGuid                  = "some-generated-guid"
		serviceInstanceGUID                 = "test-service-instance-guid"
		spaceGUID                           = "test-space-guid"
		listServiceBindingsUrl              = "/v3/service_credential_bindings"
	)

	var (
		req                 *http.Request
		serviceBindingRepo  *fake.CFServiceBindingRepository
		appRepo             *fake.CFAppRepository
		serviceInstanceRepo *fake.CFServiceInstanceRepository
	)

	BeforeEach(func() {
		serviceBindingRepo = new(fake.CFServiceBindingRepository)
		appRepo = new(fake.CFAppRepository)
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		handler := NewServiceBindingHandler(
			logf.Log.WithName(testServiceBindingHandlerLoggerName),
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		)
		handler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the POST /v3/service_credential_bindings endpoint", func() {
		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{
				GUID:      appGUID,
				SpaceGUID: spaceGUID,
			}, nil)

			serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
				GUID:      serviceInstanceGUID,
				SpaceGUID: spaceGUID,
			}, nil)

			validBody := fmt.Sprintf(`{
				"type": "app",
				"relationships": {
					"app": {
						"data": {
							"guid": %q
						}
					},
					"service_instance": {
						"data": {
							"guid": %q
						}
					}
				}
			}`, appGUID, serviceInstanceGUID)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/service_credential_bindings", strings.NewReader(validBody))
			Expect(err).NotTo(HaveOccurred())
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				req.Body = io.NopCloser(strings.NewReader(`{"description"`))
			})

			It("returns an error", func() {
				expectBadRequestError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When(`the type is "key"`, func() {
			BeforeEach(func() {
				req.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(`{
					"type": "key",
					"relationships": {
						"app": {
							"data": {
								"guid": %q
							}
						},
						"service_instance": {
							"data": {
								"guid": %q
							}
						}
					}
				}`, appGUID, serviceInstanceGUID)))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`Type must be one of [app]`)
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("all relationships are missing", func() {
			BeforeEach(func() {
				req.Body = io.NopCloser(strings.NewReader(`{ "type": "app" }`))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`Relationships is a required field`)
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the app relationship is missing", func() {
			BeforeEach(func() {
				req.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(`{
					"type": "app",
					"relationships": {
						"service_instance": {
							"data": {
								"guid": %q
							}
						}
					}
				}`, serviceInstanceGUID)))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`App is a required field`)
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the service_instance relationship is missing", func() {
			BeforeEach(func() {
				req.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(`{
					"type": "app",
					"relationships": {
						"app": {
							"data": {
								"guid": %q
							}
						}
					}
				}`, appGUID)))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("ServiceInstance is a required field")
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("a binding already exists for the App and ServiceInstance", func() {
			BeforeEach(func() {
				serviceBindingRepo.ServiceBindingExistsReturns(true, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("The app is already bound to the service instance")
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the App and the ServiceInstance are in different spaces", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{SpaceGUID: spaceGUID}, nil)
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{SpaceGUID: "another-space-guid"}, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("The service instance and the app are in different spaces")
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the user can't get Apps in the Space", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.ForbiddenError{})
			})

			It("returns an error", func() {
				expectNotAuthorizedError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the user can't get ServiceInstances in the Space", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, repositories.ForbiddenError{})
			})

			It("returns an error", func() {
				expectNotAuthorizedError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the user can't get ServiceBindings in the Space", func() {
			BeforeEach(func() {
				serviceBindingRepo.ServiceBindingExistsReturns(false, repositories.ForbiddenError{})
			})

			It("returns an error", func() {
				expectNotAuthorizedError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the user can't create ServiceBindings in the Space", func() {
			BeforeEach(func() {
				serviceBindingRepo.CreateServiceBindingReturns(repositories.ServiceBindingRecord{}, repositories.ForbiddenError{})
			})

			It("returns an error", func() {
				expectNotAuthorizedError()
			})
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, authorization.InvalidAuthError{})
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, authorization.InvalidAuthError{})
				serviceBindingRepo.ServiceBindingExistsReturns(false, authorization.InvalidAuthError{})
			})

			It("returns Invalid Auth error", func() {
				expectInvalidAuthError()
			})
		})

		When("authentication is not provided", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, authorization.NotAuthenticatedError{})
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, authorization.NotAuthenticatedError{})
				serviceBindingRepo.ServiceBindingExistsReturns(false, authorization.NotAuthenticatedError{})
			})

			It("returns a NotAuthenticated error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("getting the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("getting the ServiceInstance errors", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("checking for a duplicate ServiceBinding errors", func() {
			BeforeEach(func() {
				serviceBindingRepo.ServiceBindingExistsReturns(false, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("doesn't create the ServiceBinding", func() {
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("creating the ServiceBinding errors", func() {
			BeforeEach(func() {
				serviceBindingRepo.CreateServiceBindingReturns(repositories.ServiceBindingRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/service_credential_bindings endpoint", func() {
		BeforeEach(func() {
			serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{{
				GUID:                serviceBindingGuid,
				Type:                "app",
				AppGUID:             appGUID,
				ServiceInstanceGUID: serviceInstanceGUID,
				SpaceGUID:           spaceGUID,
				CreatedAt:           "",
				UpdatedAt:           "",
				LastOperation:       repositories.ServiceBindingLastOperation{},
			}}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", listServiceBindingsUrl, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the ServiceBindings available to the user", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring(serviceBindingGuid))
		})

		When("no service bindings can be found", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, nil)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a CF API formatted empty resource list", func() {
				Expect(rr.Body.String()).Should(MatchJSON(fmt.Sprintf(`{
				"pagination": {
				  "total_results": 0,
				  "total_pages": 1,
				  "first": {
					"href": "%[1]s/v3/service_credential_bindings"
				  },
				  "last": {
					"href": "%[1]s/v3/service_credential_bindings"
				  },
				  "next": null,
				  "previous": null
				},
				"resources": []
			}`, defaultServerURL)), "Response body matches response:")
			})
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, authorization.InvalidAuthError{})
			})

			It("returns Invalid Auth error", func() {
				expectInvalidAuthError()
			})
		})

		When("authentication is not provided", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, authorization.NotAuthenticatedError{})
			})

			It("returns a NotAuthenticated error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("user is not allowed to list a service instance", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, repositories.NewForbiddenError(repositories.ServiceBindingResourceType, errors.New("not allowed")))
			})

			It("returns an unauthorised error", func() {
				expectNotAuthorizedError()
			})
		})

		When("there is some other error fetching service instances", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, errors.New("unknown"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("an include=app query parameter is specified", func() {
			BeforeEach(func() {
				req.URL.RawQuery = "include=app"

				appRepo.ListAppsReturns([]repositories.AppRecord{{Name: "some-app-name"}}, nil)
			})

			It("calls the App repository to fetch apps from the bindings", func() {
				Expect(appRepo.ListAppsCallCount()).To(Equal(1))
				_, _, listAppsMessage := appRepo.ListAppsArgsForCall(0)
				Expect(listAppsMessage.Guids).To(ContainElements(appGUID))
			})

			It("includes app data in the response", func() {
				Expect(rr.Body.String()).To(ContainSubstring("some-app-name"))
			})
		})

		When("an app_guids query parameter is provided", func() {
			BeforeEach(func() {
				req.URL.RawQuery = "app_guids=1,2,3"

				appRepo.ListAppsReturns([]repositories.AppRecord{{Name: "some-app-name"}}, nil)
			})

			It("passes the list of app GUIDs to the repository", func() {
				Expect(serviceBindingRepo.ListServiceBindingsCallCount()).To(Equal(1))
				_, _, message := serviceBindingRepo.ListServiceBindingsArgsForCall(0)

				Expect(message.AppGUIDs).To(ConsistOf([]string{"1", "2", "3"}))
			})
		})

		When("a service_instance_guids query parameter is provided", func() {
			BeforeEach(func() {
				req.URL.RawQuery = "service_instance_guids=1,2,3"

				appRepo.ListAppsReturns([]repositories.AppRecord{{Name: "some-app-name"}}, nil)
			})

			It("passes the list of service instance GUIDs to the repository", func() {
				Expect(serviceBindingRepo.ListServiceBindingsCallCount()).To(Equal(1))
				_, _, message := serviceBindingRepo.ListServiceBindingsArgsForCall(0)
				Expect(message.ServiceInstanceGUIDs).To(ConsistOf([]string{"1", "2", "3"}))
			})

			It("does not include app data in the response", func() {
				Expect(appRepo.ListAppsCallCount()).To(Equal(0))
				Expect(rr.Body.String()).NotTo(ContainSubstring("included"))
			})
		})

		When("a type query parameter is provided", func() {
			BeforeEach(func() {
				req.URL.RawQuery = "type=app"
			})

			It("returns success", func() {
				Expect(rr.Code).To(Equal(http.StatusOK))
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				req.URL.RawQuery = "foo=bar"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'app_guids, service_instance_guids, include, type'")
			})
		})
	})
})
