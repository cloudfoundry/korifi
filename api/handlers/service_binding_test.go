package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
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
		requestValidator    *fake.RequestValidator
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

		requestValidator = new(fake.RequestValidator)

		apiHandler := NewServiceBinding(
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/service_credential_bindings", func() {
		var payload payloads.ServiceBindingCreate

		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/service_credential_bindings"
			requestBody = "the-json-body"

			serviceBindingRepo.CreateServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID: "service-binding-guid",
			}, nil)

			payload = payloads.ServiceBindingCreate{
				Relationships: &payloads.ServiceBindingRelationships{
					App: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "app-guid",
						},
					},
					ServiceInstance: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "service-instance-guid",
						},
					},
				},
				Type: "app",
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payload)
		})

		It("creates a service binding", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

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
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
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
			requestPath = "/v3/service_credential_bindings?foo=bar"

			serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{
				{GUID: "service-binding-guid", AppGUID: "app-guid"},
			}, nil)
			appRepo.ListAppsReturns([]repositories.AppRecord{{Name: "some-app-name"}}, nil)

			payload := payloads.ServiceBindingList{
				AppGUIDs:             "a1,a2",
				ServiceInstanceGUIDs: "s1,s2",
			}
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payload)
		})

		It("returns the list of ServiceBindings", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix(requestPath))

			Expect(serviceBindingRepo.ListServiceBindingsCallCount()).To(Equal(1))
			_, _, message := serviceBindingRepo.ListServiceBindingsArgsForCall(0)
			Expect(message.AppGUIDs).To(ConsistOf([]string{"a1", "a2"}))
			Expect(message.ServiceInstanceGUIDs).To(ConsistOf([]string{"s1", "s2"}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_credential_bindings?foo=bar"),
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
				payload := payloads.ServiceBindingList{
					Include: "app",
				}
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payload)
			})

			It("includes app data in the response", func() {
				Expect(appRepo.ListAppsCallCount()).To(Equal(1))
				_, _, listAppsMessage := appRepo.ListAppsArgsForCall(0)
				Expect(listAppsMessage.Guids).To(ContainElements("app-guid"))

				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.included.apps[0].name", "some-app-name")))
			})
		})

		When("decoding URL params fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
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
			requestBody = "the-json-body"

			serviceBindingRepo.UpdateServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID: "service-binding-guid",
			}, nil)

			payload := payloads.ServiceBindingUpdate{
				Metadata: payloads.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payload)
		})

		It("updates the service binding", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

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
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
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
