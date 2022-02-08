package apis_test

import (
	"errors"
	"fmt"
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
		serviceInstanceGUID                 = "test-service-instance-guid"
		spaceGUID                           = "test-space-guid"
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
		makePostRequest := func(body string, extra ...interface{}) {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/service_credential_bindings", strings.NewReader(
				fmt.Sprintf(body, extra...),
			))
			Expect(err).NotTo(HaveOccurred())
		}

		var validBody string

		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{
				GUID:      appGUID,
				SpaceGUID: spaceGUID,
			}, nil)

			serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
				GUID:      serviceInstanceGUID,
				SpaceGUID: spaceGUID,
			}, nil)

			validBody = fmt.Sprintf(`{
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
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				makePostRequest(`{"description"`)
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
				makePostRequest(`{
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
				}`, appGUID, serviceInstanceGUID)
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
				makePostRequest(`{ "type": "app" }`)
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
				makePostRequest(`{
					"type": "app",
					"relationships": {
						"service_instance": {
							"data": {
								"guid": %q
							}
						}
					}
				}`, serviceInstanceGUID)
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
				makePostRequest(`{
					"type": "app",
					"relationships": {
						"app": {
							"data": {
								"guid": %q
							}
						}
					}
				}`, appGUID)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
			})

			It("returns a NotAuthenticated error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("getting the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
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
				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
