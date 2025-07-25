package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceInstance", func() {
	var (
		serviceInstanceRepo *fake.CFServiceInstanceRepository
		spaceRepo           *fake.CFSpaceRepository
		orgRepo             *fake.CFOrgRepository
		serviceOfferingRepo *fake.CFServiceOfferingRepository
		servicePlanRepo     *fake.CFServicePlanRepository
		serviceBrokerRepo   *fake.CFServiceBrokerRepository
		requestValidator    *fake.RequestValidator

		reqMethod string
		reqPath   string
	)

	BeforeEach(func() {
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
			GUID:      "service-instance-guid",
			SpaceGUID: "space-guid",
			Type:      korifiv1alpha1.UserProvidedType,
		}, nil)

		spaceRepo = new(fake.CFSpaceRepository)
		orgRepo = new(fake.CFOrgRepository)
		serviceBrokerRepo = new(fake.CFServiceBrokerRepository)
		serviceOfferingRepo = new(fake.CFServiceOfferingRepository)
		servicePlanRepo = new(fake.CFServicePlanRepository)

		requestValidator = new(fake.RequestValidator)

		apiHandler := NewServiceInstance(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			requestValidator,
			relationships.NewResourseRelationshipsRepo(
				serviceOfferingRepo,
				serviceBrokerRepo,
				servicePlanRepo,
				spaceRepo,
				orgRepo,
			),
		)
		routerBuilder.LoadRoutes(apiHandler)

		reqMethod = http.MethodGet
		reqPath = "/v3/service_instances"
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, reqMethod, reqPath, strings.NewReader("the-json-body"))
		Expect(err).NotTo(HaveOccurred())
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/service_instances/:guid", func() {
		BeforeEach(func() {
			serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
				GUID:      "service-instance-guid",
				Type:      korifiv1alpha1.UserProvidedType,
				SpaceGUID: "space-guid",
			}, nil)

			reqPath += "/service-instance-guid"
		})

		It("gets the service instance", func() {
			Expect(serviceInstanceRepo.GetServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, actualServiceInstanceGUID := serviceInstanceRepo.GetServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualServiceInstanceGUID).To(Equal("service-instance-guid"))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "service-instance-guid"),
				MatchJSONPath("$.type", "user-provided"),
			)))
		})

		When("getting the service instance fails with an error", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("getting the service instance fails with forbidden", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewForbiddenError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns an 404 Not Found error", func() {
				expectNotFoundError("Service Instance")
			})
		})

		When("the query is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		Describe("fields", func() {
			BeforeEach(func() {
				serviceOfferingRepo.ListOfferingsReturns(repositories.ListResult[repositories.ServiceOfferingRecord]{
					Records: []repositories.ServiceOfferingRecord{{
						Name:              "service-offering-name",
						GUID:              "service-offering-guid",
						ServiceBrokerGUID: "service-broker-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     1,
					},
				}, nil)

				servicePlanRepo.ListPlansReturns(repositories.ListResult[repositories.ServicePlanRecord]{
					Records: []repositories.ServicePlanRecord{{
						Name:                "service-plan-name",
						GUID:                "service-plan-guid",
						ServiceOfferingGUID: "service-offering-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     1,
					},
				}, nil)

				serviceBrokerRepo.ListServiceBrokersReturns(repositories.ListResult[repositories.ServiceBrokerRecord]{
					Records: []repositories.ServiceBrokerRecord{{
						Name: "service-broker-name",
						GUID: "service-broker-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
					},
				}, nil)

				orgRepo.ListOrgsReturns(repositories.ListResult[repositories.OrgRecord]{
					Records: []repositories.OrgRecord{{
						Name: "org-name",
						GUID: "org-guid",
					}},
				}, nil)

				spaceRepo.ListSpacesReturns(repositories.ListResult[repositories.SpaceRecord]{
					Records: []repositories.SpaceRecord{{
						Name:             "space-name",
						OrganizationGUID: "org-guid",
						GUID:             "space-guid",
					}},
					PageInfo: descriptors.PageInfo{TotalResults: 1},
				}, nil)
			})

			When("params to inlude fields[service_plan]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceGet{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_plan"},
							Fields:           []string{"name", "guid"},
						}},
					})
				})

				It("does not include resources", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(Not(ContainSubstring("included"))))
				})

				When("the service instance is managed", func() {
					BeforeEach(func() {
						serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
							GUID: "service-instance-guid", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid",
						}, nil)
					})

					It("includes offering fields in the response", func() {
						Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
						Expect(rr).To(HaveHTTPBody(SatisfyAll(
							MatchJSONPath("$.included.service_plans[0].guid", "service-plan-guid"),
							MatchJSONPath("$.included.service_plans[0].name", "service-plan-name"),
						)))
					})
				})
			})

			When("params to inlude fields[service_plan.service_offering]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceGet{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_plan", "service_offering"},
							Fields:           []string{"name", "guid", "relationships.service_broker"},
						}},
					})
				})

				It("does not include resources", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(Not(ContainSubstring("included"))))
				})

				When("the service instance is managed", func() {
					BeforeEach(func() {
						serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
							GUID: "service-instance-guid", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid",
						}, nil)
					})

					It("includes offering fields in the response", func() {
						Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
						Expect(rr).To(HaveHTTPBody(SatisfyAll(
							MatchJSONPath("$.included.service_offerings[0].guid", "service-offering-guid"),
							MatchJSONPath("$.included.service_offerings[0].name", "service-offering-name"),
							MatchJSONPath("$.included.service_offerings[0].relationships.service_broker.data.guid", "service-broker-guid"),
						)))
					})
				})
			})

			When("params to inlude fields[service_plan.service_offering.service_broker]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceGet{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_plan", "service_offering", "service_broker"},
							Fields:           []string{"name", "guid"},
						}},
					})
				})

				It("does not include resources", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(Not(ContainSubstring("included"))))
				})

				When("the service instance is managed", func() {
					BeforeEach(func() {
						serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
							GUID: "service-instance-guid", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid",
						}, nil)
					})

					It("includes broker fields in the response", func() {
						Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
						Expect(rr).To(HaveHTTPBody(SatisfyAll(
							MatchJSONPath("$.included.service_brokers[0].guid", "service-broker-guid"),
							MatchJSONPath("$.included.service_brokers[0].name", "service-broker-name"),
						)))
					})
				})
			})

			When("params to inlude fields[space]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceGet{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"space"},
							Fields:           []string{"name", "guid"},
						}},
					})
				})

				It("includes space in the response", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.included.spaces[0].guid", "space-guid"),
						MatchJSONPath("$.included.spaces[0].name", "space-name"),
					)))
				})
			})

			When("params to inlude fields[space.organization]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceGet{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"space", "organization"},
							Fields:           []string{"name", "guid"},
						}},
					})
				})

				It("includes space in the response", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.included.organizations[0].guid", "org-guid"),
						MatchJSONPath("$.included.organizations[0].name", "org-name"),
					)))
				})
			})
		})
	})

	Describe("GET /v3/service_instances/:guid/credentials", func() {
		BeforeEach(func() {
			serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
				GUID: "service-instance-guid",
				Type: korifiv1alpha1.UserProvidedType,
			}, nil)

			serviceInstanceRepo.GetServiceInstanceCredentialsReturns(map[string]any{
				"foo": "bar",
			}, nil)

			reqPath += "/service-instance-guid/credentials"
		})

		It("gets the service instance credentials", func() {
			Expect(serviceInstanceRepo.GetServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, actualServiceInstanceGUID := serviceInstanceRepo.GetServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualServiceInstanceGUID).To(Equal("service-instance-guid"))

			Expect(serviceInstanceRepo.GetServiceInstanceCredentialsCallCount()).To(Equal(1))
			_, actualAuthInfo, actualInstanceGUID := serviceInstanceRepo.GetServiceInstanceCredentialsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualInstanceGUID).To(Equal("service-instance-guid"))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.foo", "bar"),
			)))
		})

		When("the service instance is not user-provided", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
					GUID: "service-instance-guid",
					Type: korifiv1alpha1.ManagedType,
				}, nil)
			})

			It("returns a 404 Not Found error", func() {
				expectNotFoundError("Service Instance")
			})
		})

		When("the service instance does not have credentials", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
					GUID: "service-instance-guid",
					Type: korifiv1alpha1.UserProvidedType,
				}, nil)

				serviceInstanceRepo.GetServiceInstanceCredentialsReturns(map[string]any{}, apierrors.NewNotFoundError(nil, repositories.ServiceInstanceResourceType))
			})

			It("returns an 404 Not Found error", func() {
				expectNotFoundError("Service Instance")
			})
		})

		When("getting the service instance fails with an error", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					errors.New("boom"),
				)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("getting the service instance fails with forbidden", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewForbiddenError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns 404 Not Found", func() {
				expectNotFoundError("Service Instance")
			})
		})
	})

	Describe("the POST /v3/service_instances endpoint", func() {
		BeforeEach(func() {
			reqMethod = http.MethodPost

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServiceInstanceCreate{
				Relationships: &payloads.ServiceInstanceRelationships{
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "space-guid",
						},
					},
				},
			})
		})

		It("validates the request", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		When("the request body is not valid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "nope"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("nope")
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(
					repositories.SpaceRecord{},
					apierrors.NewNotFoundError(errors.New("not found"), repositories.SpaceResourceType),
				)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("the get space returns an unknown error", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(
					repositories.SpaceRecord{},
					errors.New("unknown"),
				)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("creating a user provided serivce instance", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServiceInstanceCreate{
					Name: "service-instance-name",
					Type: "user-provided",
					Relationships: &payloads.ServiceInstanceRelationships{
						Space: &payloads.Relationship{
							Data: &payloads.RelationshipData{
								GUID: "space-guid",
							},
						},
					},
				})

				serviceInstanceRepo.CreateUserProvidedServiceInstanceReturns(repositories.ServiceInstanceRecord{GUID: "service-instance-guid"}, nil)
			})

			It("creates a user provided service instance with the repository", func() {
				Expect(serviceInstanceRepo.CreateUserProvidedServiceInstanceCallCount()).To(Equal(1))
				_, actualAuthInfo, actualCreate := serviceInstanceRepo.CreateUserProvidedServiceInstanceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualCreate).To(Equal(repositories.CreateUPSIMessage{
					Name:      "service-instance-name",
					SpaceGUID: "space-guid",
				}))
			})

			When("creating the service instance fails", func() {
				BeforeEach(func() {
					serviceInstanceRepo.CreateUserProvidedServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("space-instance-creation-failed"))
				})

				It("returns unknown error", func() {
					expectUnknownError()
				})
			})

			It("returns HTTP 201 Created response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.guid", "service-instance-guid"),
					MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_instances/service-instance-guid"),
				)))
			})
		})

		When("the service instance is managed", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServiceInstanceCreate{
					Name: "service-instance-name",
					Type: "managed",
					Relationships: &payloads.ServiceInstanceRelationships{
						Space: &payloads.Relationship{
							Data: &payloads.RelationshipData{
								GUID: "space-guid",
							},
						},
						ServicePlan: &payloads.Relationship{
							Data: &payloads.RelationshipData{
								GUID: "plan-guid",
							},
						},
					},
				})

				serviceInstanceRepo.CreateManagedServiceInstanceReturns(repositories.ServiceInstanceRecord{GUID: "service-instance-guid"}, nil)
			})

			It("creates a managed service instance with the repository", func() {
				Expect(serviceInstanceRepo.CreateManagedServiceInstanceCallCount()).To(Equal(1))
				_, actualAuthInfo, actualCreate := serviceInstanceRepo.CreateManagedServiceInstanceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualCreate).To(Equal(repositories.CreateManagedSIMessage{
					Name:      "service-instance-name",
					SpaceGUID: "space-guid",
					PlanGUID:  "plan-guid",
				}))
			})

			When("creating the managed service instance fails", func() {
				BeforeEach(func() {
					serviceInstanceRepo.CreateManagedServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("create-managed-err"))
				})

				It("returns unknown error", func() {
					expectUnknownError()
				})
			})

			It("returns HTTP 202 Accepted response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
				Expect(rr).To(HaveHTTPHeaderWithValue("Location",
					ContainSubstring("/v3/jobs/managed_service_instance.create~service-instance-guid")))
			})
		})
	})

	Describe("GET /v3/service_instances", func() {
		BeforeEach(func() {
			serviceInstanceRepo.ListServiceInstancesReturns(
				repositories.ListResult[repositories.ServiceInstanceRecord]{
					PageInfo: descriptors.PageInfo{TotalResults: 2},
					Records: []repositories.ServiceInstanceRecord{
						{GUID: "service-inst-guid-1", SpaceGUID: "space-guid"},
						{GUID: "service-inst-guid-2", SpaceGUID: "space-guid"},
					},
				}, nil)

			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceList{})
			reqPath += "?foo=bar"
		})

		It("lists the service instances", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix("/v3/service_instances?foo=bar"))

			Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
			_, actualAuthInfo, actualListMessage := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualListMessage.Names).To(BeEmpty())
			Expect(actualListMessage.SpaceGUIDs).To(BeEmpty())
			Expect(actualListMessage.Pagination).To(Equal(repositories.Pagination{PerPage: 50, Page: 1}))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)),
				MatchJSONPath("$.resources[0].guid", "service-inst-guid-1"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_instances/service-inst-guid-1"),
				MatchJSONPath("$.resources[1].guid", "service-inst-guid-2"),
			)))
		})

		When("filtering query parameters are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceList{
					Names:         "sc1,sc2",
					SpaceGUIDs:    "space1,space2",
					GUIDs:         "g1,g2",
					PlanGUIDs:     "p1,p2",
					LabelSelector: "label=value",
					OrderBy:       "created_at",
					Pagination: payloads.Pagination{
						PerPage: "16",
						Page:    "32",
					},
				})
			})

			It("passes them to the repository", func() {
				Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
				_, _, message := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)

				Expect(message).To(Equal(repositories.ListServiceInstanceMessage{
					Pagination: repositories.Pagination{
						PerPage: 16,
						Page:    32,
					},
					Names:         []string{"sc1", "sc2"},
					SpaceGUIDs:    []string{"space1", "space2"},
					GUIDs:         []string{"g1", "g2"},
					LabelSelector: "label=value",
					OrderBy:       "created_at",
					PlanGUIDs:     []string{"p1", "p2"},
				}))
			})
		})

		Describe("fields", func() {
			BeforeEach(func() {
				serviceOfferingRepo.ListOfferingsReturns(repositories.ListResult[repositories.ServiceOfferingRecord]{
					Records: []repositories.ServiceOfferingRecord{{
						Name:              "service-offering-name",
						GUID:              "service-offering-guid",
						ServiceBrokerGUID: "service-broker-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     1,
					},
				}, nil)

				servicePlanRepo.ListPlansReturns(repositories.ListResult[repositories.ServicePlanRecord]{
					Records: []repositories.ServicePlanRecord{{
						Name:                "service-plan-name",
						GUID:                "service-plan-guid",
						ServiceOfferingGUID: "service-offering-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     1,
					},
				}, nil)

				serviceBrokerRepo.ListServiceBrokersReturns(
					repositories.ListResult[repositories.ServiceBrokerRecord]{
						PageInfo: descriptors.PageInfo{TotalResults: 1},
						Records: []repositories.ServiceBrokerRecord{{
							Name: "service-broker-name",
							GUID: "service-broker-guid",
						}},
					}, nil)

				orgRepo.ListOrgsReturns(repositories.ListResult[repositories.OrgRecord]{
					Records: []repositories.OrgRecord{{
						Name: "org-name",
						GUID: "org-guid",
					}},
				}, nil)

				spaceRepo.ListSpacesReturns(repositories.ListResult[repositories.SpaceRecord]{
					Records: []repositories.SpaceRecord{{
						Name:             "space-name",
						OrganizationGUID: "org-guid",
						GUID:             "space-guid",
					}},
					PageInfo: descriptors.PageInfo{TotalResults: 1},
				}, nil)
			})

			When("params to inlude fields[service_plan.service_offering]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceList{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_plan", "service_offering"},
							Fields:           []string{"name", "guid", "relationships.service_broker"},
						}},
					})
				})

				It("does not include resources", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(Not(ContainSubstring("included"))))
				})

				When("the service instance is managed", func() {
					BeforeEach(func() {
						serviceInstanceRepo.ListServiceInstancesReturns(
							repositories.ListResult[repositories.ServiceInstanceRecord]{
								PageInfo: descriptors.PageInfo{TotalResults: 2},
								Records: []repositories.ServiceInstanceRecord{
									{GUID: "service-inst-guid-1", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid"},
									{GUID: "service-inst-guid-2", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid"},
								},
							}, nil)
					})

					It("includes offering fields in the response", func() {
						Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
						Expect(rr).To(HaveHTTPBody(SatisfyAll(
							MatchJSONPath("$.included.service_offerings[0].guid", "service-offering-guid"),
							MatchJSONPath("$.included.service_offerings[0].name", "service-offering-name"),
							MatchJSONPath("$.included.service_offerings[0].relationships.service_broker.data.guid", "service-broker-guid"),
						)))
					})
				})
			})

			When("params to inlude fields[service_plan.service_offering.service_broker]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceList{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_plan", "service_offering", "service_broker"},
							Fields:           []string{"name", "guid"},
						}},
					})
				})

				It("does not include resources", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(Not(ContainSubstring("included"))))
				})

				When("the service instance is managed", func() {
					BeforeEach(func() {
						serviceInstanceRepo.ListServiceInstancesReturns(
							repositories.ListResult[repositories.ServiceInstanceRecord]{
								PageInfo: descriptors.PageInfo{TotalResults: 2},
								Records: []repositories.ServiceInstanceRecord{
									{GUID: "service-inst-guid-1", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid"},
									{GUID: "service-inst-guid-2", Type: korifiv1alpha1.ManagedType, PlanGUID: "service-plan-guid"},
								},
							}, nil)
					})

					It("includes broker fields in the response", func() {
						Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
						Expect(rr).To(HaveHTTPBody(SatisfyAll(
							MatchJSONPath("$.included.service_brokers[0].guid", "service-broker-guid"),
							MatchJSONPath("$.included.service_brokers[0].name", "service-broker-name"),
						)))
					})
				})
			})

			When("params to inlude fields[space]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceList{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"space"},
							Fields:           []string{"name", "guid", "relationships.organization"},
						}},
					})
				})

				It("includes space in the response", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.included.spaces[0].guid", "space-guid"),
						MatchJSONPath("$.included.spaces[0].name", "space-name"),
						MatchJSONPath("$.included.spaces[0].relationships.organization.data.guid", "org-guid"),
					)))
				})
			})

			When("params to inlude fields[space.organization]", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceList{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"space", "organization"},
							Fields:           []string{"name", "guid"},
						}},
					})
				})

				It("includes space in the response", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.included.organizations[0].guid", "org-guid"),
						MatchJSONPath("$.included.organizations[0].name", "org-name"),
					)))
				})
			})
		})

		When("there is an error fetching service instances", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns(repositories.ListResult[repositories.ServiceInstanceRecord]{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the query is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("PATCH /v3/service_instances/:guid", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServiceInstancePatch{
				Name:        tools.PtrTo("new-name"),
				Tags:        &[]string{"alice", "bob"},
				Credentials: &map[string]any{"foo": "bar"},
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{"ann2": tools.PtrTo("ann_val2")},
					Labels:      map[string]*string{"lab2": tools.PtrTo("lab_val2")},
				},
			})

			serviceInstanceRepo.PatchServiceInstanceReturns(repositories.ServiceInstanceRecord{
				Name: "new-name",
				GUID: "service-instance-guid",
			}, nil)

			reqPath += "/service-instance-guid"
			reqMethod = http.MethodPatch
		})

		It("patches the service instance", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(serviceInstanceRepo.GetServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, actualGUID := serviceInstanceRepo.GetServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualGUID).To(Equal("service-instance-guid"))

			Expect(serviceInstanceRepo.PatchServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, patchMessage := serviceInstanceRepo.PatchServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(patchMessage).To(Equal(repositories.PatchServiceInstanceMessage{
				GUID:        "service-instance-guid",
				SpaceGUID:   "space-guid",
				Name:        tools.PtrTo("new-name"),
				Credentials: &map[string]any{"foo": "bar"},
				Tags:        &[]string{"alice", "bob"},
				MetadataPatch: repositories.MetadataPatch{
					Annotations: map[string]*string{"ann2": tools.PtrTo("ann_val2")},
					Labels:      map[string]*string{"lab2": tools.PtrTo("lab_val2")},
				},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "service-instance-guid"),
				MatchJSONPath("$.name", "new-name"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_instances/service-instance-guid"),
			)))
		})

		When("decoding the payload fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "nope"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("nope")
			})
		})

		When("getting the service instance fails with not found", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewNotFoundError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns 404 Not Found", func() {
				expectNotFoundError("Service Instance")
			})
		})

		When("getting the service instance fails with forbidden", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewForbiddenError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns 404 Not Found", func() {
				expectNotFoundError("Service Instance")
			})
		})

		When("patching the service instances fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.PatchServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("oops"))
			})

			It("returns the error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("DELETE /v3/service_instances/:guid", func() {
		BeforeEach(func() {
			reqPath += "/service-instance-guid"
			reqMethod = http.MethodDelete
		})

		It("deletes the service instance", func() {
			Expect(serviceInstanceRepo.DeleteServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, message := serviceInstanceRepo.DeleteServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(message.GUID).To(Equal("service-instance-guid"))
			Expect(message.Purge).To(BeFalse())

			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
		})

		When("the service instance is managed", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(repositories.ServiceInstanceRecord{
					GUID:      "service-instance-guid",
					SpaceGUID: "space-guid",
					Type:      korifiv1alpha1.ManagedType,
				}, nil)
			})

			It("deletes the service instance", func() {
				Expect(serviceInstanceRepo.DeleteServiceInstanceCallCount()).To(Equal(1))
				_, actualAuthInfo, message := serviceInstanceRepo.DeleteServiceInstanceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(message.GUID).To(Equal("service-instance-guid"))
				Expect(message.Purge).To(BeFalse())

				Expect(rr).To(SatisfyAll(
					HaveHTTPStatus(http.StatusAccepted),
					HaveHTTPHeaderWithValue("Location", ContainSubstring("/v3/jobs/managed_service_instance.delete~service-instance-guid")),
				))
			})
		})

		When("purging is set to true", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceInstanceDelete{
					Purge: true,
				})
			})

			It("purges the service instance", func() {
				Expect(serviceInstanceRepo.DeleteServiceInstanceCallCount()).To(Equal(1))
				_, actualAuthInfo, message := serviceInstanceRepo.DeleteServiceInstanceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(message.GUID).To(Equal("service-instance-guid"))
				Expect(message.Purge).To(BeTrue())

				Expect(rr).To(SatisfyAll(
					HaveHTTPStatus(http.StatusNoContent),
				))
			})
		})

		When("getting the service instance fails with not found", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewNotFoundError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns 404 Not Found", func() {
				expectNotFoundError("Service Instance")
			})
		})

		When("getting the service instance fails with forbidden", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewForbiddenError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns 403 Forbidden", func() {
				expectNotAuthorizedError()
			})
		})

		When("getting the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				expectUnknownError()
			})
		})

		When("deleting the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				expectUnknownError()
			})
		})
	})
})
