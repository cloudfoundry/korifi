package handlers_test

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceBinding", func() {
	var (
		requestMethod string
		requestPath   string
		requestBody   string

		serviceBindingRepo  *fake.CFServiceBindingRepository
		appRepo             *fake.CFAppRepository
		serviceInstanceRepo *fake.CFServiceInstanceRepository
	)

	BeforeEach(func() {
		serviceBindingRepo = new(fake.CFServiceBindingRepository)
		serviceBindingRepo.GetServiceBindingReturns(repositories.ServiceBindingRecord{
			GUID: "service-binding-guid",
		}, nil)

		appRepo = new(fake.CFAppRepository)
		appRepo.GetAppReturns(repositories.AppRecord{
			GUID:      "app-guid",
			SpaceGUID: "space-guid",
		}, nil)

		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
			GUID:      "service-instance-guid",
			SpaceGUID: "space-guid",
		}, nil)

		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewServiceBinding(
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/service_credential_bindings", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/service_credential_bindings"
			requestBody = `{
				"type": "app",
				"relationships": {
					"app": {
						"data": {
							"guid": "app-guid"
						}
					},
					"service_instance": {
						"data": {
							"guid": "service-instance-guid"
						}
					}
				}
			}`

			serviceBindingRepo.CreateServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID: "service-binding-guid",
			}, nil)
		})

		It("creates a service binding", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppGUID).To(Equal("app-guid"))

			Expect(serviceInstanceRepo.GetServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, actualServiceInstanceGUID := serviceInstanceRepo.GetServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualServiceInstanceGUID).To(Equal("service-instance-guid"))

			Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(1))
			_, actualAuthInfo, createServiceBindingMessage := serviceBindingRepo.CreateServiceBindingArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createServiceBindingMessage.AppGUID).To(Equal("app-guid"))
			Expect(createServiceBindingMessage.ServiceInstanceGUID).To(Equal("service-instance-guid"))
			Expect(createServiceBindingMessage.SpaceGUID).To(Equal("space-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "service-binding-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_credential_bindings/service-binding-guid"),
			)))
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				requestBody = "{"
			})

			It("returns an error and doesn't create the service binding", func() {
				expectBadRequestError()
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When(`the type is "key"`, func() {
			BeforeEach(func() {
				requestBody = `{
					"type": "key",
					"relationships": {
						"app": {
							"data": {
								"guid": "app-guid"
							}
						},
						"service_instance": {
							"data": {
								"guid": "service-instance-guid"
							}
						}
					}
				}`
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnprocessableEntityError(regexp.QuoteMeta("Type must be one of [app]"))
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("all relationships are missing", func() {
			BeforeEach(func() {
				requestBody = `{ "type": "app" }`
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnprocessableEntityError(`Relationships is a required field`)
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the app relationship is missing", func() {
			BeforeEach(func() {
				requestBody = `{
					"type": "app",
					"relationships": {
						"service_instance": {
							"data": {
								"guid": "service-instance-guid"
							}
						}
					}
				}`
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnprocessableEntityError(`App is a required field`)
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the service_instance relationship is missing", func() {
			BeforeEach(func() {
				requestBody = `{
					"type": "app",
					"relationships": {
						"app": {
							"data": {
								"guid": "app-guid"
							}
						}
					}
				}`
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnprocessableEntityError("ServiceInstance is a required field")
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("the App and the ServiceInstance are in different spaces", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{SpaceGUID: spaceGUID}, nil)
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{SpaceGUID: "another-space-guid"}, nil)
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnprocessableEntityError("The service instance and the app are in different spaces")
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("getting the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnknownError()
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(0))
			})
		})

		When("getting the ServiceInstance errors", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns an error and doesn't create the ServiceBinding", func() {
				expectUnknownError()
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

	Describe("GET /v3/service_credential_bindings/{guid}", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestPath = "/v3/service_credential_bindings/service-binding-guid"
			requestBody = ""
		})

		It("returns the service binding", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "service-binding-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_credential_bindings/service-binding-guid"),
			)))
		})

		When("the service bindding repo returns an error", func() {
			BeforeEach(func() {
				serviceBindingRepo.GetServiceBindingReturns(repositories.ServiceBindingRecord{}, errors.New("get-service-binding-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user is not authorized", func() {
			BeforeEach(func() {
				serviceBindingRepo.GetServiceBindingReturns(repositories.ServiceBindingRecord{}, apierrors.NewForbiddenError(nil, "CFServiceBinding"))
			})

			It("returns 404 NotFound", func() {
				expectNotFoundError("CFServiceBinding")
			})
		})
	})

	Describe("GET /v3/service_credential_bindings", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestBody = ""
			requestPath = "/v3/service_credential_bindings"

			serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{
				{GUID: "service-binding-guid", AppGUID: "app-guid"},
			}, nil)
			appRepo.ListAppsReturns([]repositories.AppRecord{{Name: "some-app-name"}}, nil)
		})

		It("returns the list of ServiceBindings", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_credential_bindings"),
				MatchJSONPath("$.resources[0].guid", "service-binding-guid"),
			)))
		})

		When("there is an error fetching service instances", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, errors.New("unknown"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("an include=app query parameter is specified", func() {
			BeforeEach(func() {
				requestPath += "?include=app"
			})

			It("includes app data in the response", func() {
				Expect(appRepo.ListAppsCallCount()).To(Equal(1))
				_, _, listAppsMessage := appRepo.ListAppsArgsForCall(0)
				Expect(listAppsMessage.Guids).To(ContainElements("app-guid"))

				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.included.apps[0].name", "some-app-name")))
			})
		})

		When("an app_guids query parameter is provided", func() {
			BeforeEach(func() {
				requestPath += "?app_guids=1,2,3"
			})

			It("passes the list of app GUIDs to the repository", func() {
				Expect(serviceBindingRepo.ListServiceBindingsCallCount()).To(Equal(1))
				_, _, message := serviceBindingRepo.ListServiceBindingsArgsForCall(0)
				Expect(message.AppGUIDs).To(ConsistOf([]string{"1", "2", "3"}))
			})
		})

		When("a service_instance_guids query parameter is provided", func() {
			BeforeEach(func() {
				requestPath += "?service_instance_guids=1,2,3"
			})

			It("passes the list of service instance GUIDs to the repository", func() {
				Expect(serviceBindingRepo.ListServiceBindingsCallCount()).To(Equal(1))
				_, _, message := serviceBindingRepo.ListServiceBindingsArgsForCall(0)
				Expect(message.ServiceInstanceGUIDs).To(ConsistOf([]string{"1", "2", "3"}))
			})
		})

		When("a type query parameter is provided", func() {
			BeforeEach(func() {
				requestPath += "?type=app"
			})

			It("returns success", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				requestPath += "?foo=bar"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: .*")
			})
		})
	})

	Describe("DELETE /v3/service_credential_bindings/:guid", func() {
		BeforeEach(func() {
			requestMethod = "DELETE"
			requestPath = "/v3/service_credential_bindings/service-binding-guid"
		})

		It("deletes the service binding", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
			Expect(rr).To(HaveHTTPBody(BeEmpty()))

			Expect(serviceBindingRepo.DeleteServiceBindingCallCount()).To(Equal(1))
			_, _, guid := serviceBindingRepo.DeleteServiceBindingArgsForCall(0)
			Expect(guid).To(Equal("service-binding-guid"))
		})
	})

	Describe("PATCH /v3/service_credential_bindings/:guid", func() {
		BeforeEach(func() {
			requestMethod = "PATCH"
			requestPath = "/v3/service_credential_bindings/service-binding-guid"
			requestBody = `{
				"metadata": {
					"labels": { "foo": "bar" },
					"annotations": { "bar": "baz" }
				}
			}`

			serviceBindingRepo.UpdateServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID: "service-binding-guid",
			}, nil)
		})

		It("updates the service binding", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "service-binding-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_credential_bindings/service-binding-guid"),
			)))

			Expect(serviceBindingRepo.UpdateServiceBindingCallCount()).To(Equal(1))
			_, actualAuthInfo, updateMessage := serviceBindingRepo.UpdateServiceBindingArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(updateMessage).To(Equal(repositories.UpdateServiceBindingMessage{
				GUID: "service-binding-guid",
				MetadataPatch: repositories.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}))
		})

		When("the payload cannot be decoded", func() {
			BeforeEach(func() {
				requestBody = `{"one":"two"}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("invalid request body: json: unknown field \"one\"")
			})
		})

		When("a label is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					requestBody = `{
						"metadata": {
							"labels": { "cloudfoundry.org/test": "production" }
						}
					}`
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})

			When("the prefix is a subdomain of cloudfoundry.org", func() {
				BeforeEach(func() {
					requestBody = `{
						"metadata": {
							"labels": { "korifi.cloudfoundry.org/test": "production" }
						}
					}`
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})
		})

		When("an annotation is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					requestBody = `{
						"metadata": {
							"annotations": { "cloudfoundry.org/test": "production" }
						}
					}`
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})

			When("the prefix is a subdomain of cloudfoundry.org", func() {
				BeforeEach(func() {
					requestBody = `{
						"metadata": {
							"annotations": { "korifi.cloudfoundry.org/test": "production" }
						}
					}`
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})
		})

		When("the service binding repo returns an error", func() {
			BeforeEach(func() {
				serviceBindingRepo.UpdateServiceBindingReturns(repositories.ServiceBindingRecord{}, errors.New("update-sb-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user is not authorized to get service bindings", func() {
			BeforeEach(func() {
				serviceBindingRepo.GetServiceBindingReturns(repositories.ServiceBindingRecord{}, apierrors.NewForbiddenError(nil, "CFServiceBinding"))
			})

			It("returns 404 NotFound", func() {
				expectNotFoundError("CFServiceBinding")
			})
		})
	})
})
