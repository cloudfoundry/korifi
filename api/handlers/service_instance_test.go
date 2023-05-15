package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

var _ = Describe("ServiceInstance", func() {
	var (
		serviceInstanceRepo *fake.CFServiceInstanceRepository
		spaceRepo           *fake.SpaceRepository
		payloadValidator    *fake.PayloadValidator
		payload             any

		reqMethod string
		reqPath   string
	)

	BeforeEach(func() {
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{
			GUID:      "service-instance-guid",
			SpaceGUID: "space-guid",
		}, nil)

		spaceRepo = new(fake.SpaceRepository)

		payloadValidator = new(fake.PayloadValidator)
		payloadValidator.ValidatePayloadStub = func(i interface{}) error {
			switch t := i.(type) {
			case *payloads.ServiceInstanceCreate:
				*t = *(payload.(*payloads.ServiceInstanceCreate))
			case *payloads.ServiceInstancePatch:
				*t = *(payload.(*payloads.ServiceInstancePatch))
			default:
				Fail(fmt.Sprintf("no case for payload type %T", i))
			}
			return nil
		}

		apiHandler := NewServiceInstance(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			payloadValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)

		reqMethod = http.MethodGet
		reqPath = "/v3/service_instances"
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, reqMethod, reqPath, strings.NewReader(`{"input": "json"}`))
		Expect(err).NotTo(HaveOccurred())
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the POST /v3/service_instances endpoint", func() {
		BeforeEach(func() {
			reqMethod = http.MethodPost

			payload = &payloads.ServiceInstanceCreate{
				Name: "service-instance-name",
				Type: "user-provided",
				Tags: []string{"foo", "bar"},
				Relationships: payloads.ServiceInstanceRelationships{
					Space: payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "space-guid",
						},
					},
				},
				Metadata: payloads.Metadata{},
			}

			serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{
				Name:       "service-instance-name",
				GUID:       "service-instance-guid",
				SpaceGUID:  "space-guid",
				SecretName: "secret-name",
				Tags:       []string{"foo", "bar"},
				Type:       "user-provided",
				CreatedAt:  "then",
				UpdatedAt:  "now",
			}, nil)
		})

		It("creates a CFServiceInstance", func() {
			// TODO: fix me

			Expect(serviceInstanceRepo.CreateServiceInstanceCallCount()).To(Equal(1))
			_, actualAuthInfo, actualCreate := serviceInstanceRepo.CreateServiceInstanceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualCreate).To(Equal(repositories.CreateServiceInstanceMessage{
				Name:      "service-instance-name",
				SpaceGUID: "space-guid",
				Type:      "user-provided",
				Tags:      []string{"foo", "bar"},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "service-instance-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_instances/service-instance-guid"),
			)))
		})

		When("the request body is not valid", func() {
			BeforeEach(func() {
				payloadValidator.ValidatePayloadReturns(apierrors.NewUnprocessableEntityError(nil, "nope"))
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

		When("creating the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("space-instance-creation-failed"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/service_instances", func() {
		BeforeEach(func() {
			serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{
				{
					Name:       "service-inst-name-1",
					GUID:       "service-inst-guid-1",
					SpaceGUID:  "space-guid",
					SecretName: "secret-name-1",
					Tags:       []string{"foo", "bar"},
					Type:       "user-provided",
					Labels: map[string]string{
						"a-label": "a-label-value",
					},
					Annotations: map[string]string{
						"an-annotation": "an-annotation-value",
					},
					CreatedAt: "1906-04-18T13:12:00Z",
					UpdatedAt: "1906-04-18T13:12:00Z",
				},
				{
					Name:       "service-inst-name-2",
					GUID:       "service-inst-guid-2",
					SpaceGUID:  "space-guid",
					SecretName: "secret-name-2",
					Tags:       nil,
					Type:       "user-provided",
					CreatedAt:  "1906-04-18T13:12:00Z",
					UpdatedAt:  "1906-04-18T13:12:01Z",
				},
			}, nil)
		})

		It("lists the service instances", func() {
			Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
			_, actualAuthInfo, actualListMessage := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualListMessage.Names).To(BeEmpty())
			Expect(actualListMessage.SpaceGuids).To(BeEmpty())

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_instances"),
				MatchJSONPath("$.resources[0].guid", "service-inst-guid-1"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_instances/service-inst-guid-1"),
				MatchJSONPath("$.resources[1].guid", "service-inst-guid-2"),
			)))
		})

		When("filtering query parameters are provided", func() {
			BeforeEach(func() {
				reqPath += "?" +
					"names=sc1,sc2&" +
					"space_guids=space1,space2&" +
					"fields%5Bservice_plan.service_offering.service_broker%5D=guid%2Cname"
			})

			It("passes them to the repository", func() {
				Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
				_, _, message := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)

				Expect(message.Names).To(ConsistOf("sc1", "sc2"))
				Expect(message.SpaceGuids).To(ConsistOf("space1", "space2"))
			})

			It("correctly sets query parameters in response pagination links", func() {
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_instances?names=sc1,sc2&space_guids=space1,space2&fields%5Bservice_plan.service_offering.service_broker%5D=guid%2Cname")))
			})
		})

		Describe("Order results", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{
					{
						GUID:      "1",
						Name:      "first-test-si",
						CreatedAt: "2023-01-17T14:58:32Z",
						UpdatedAt: "2023-01-18T14:58:32Z",
					},
					{
						GUID:      "2",
						Name:      "second-test-si",
						CreatedAt: "2023-01-17T14:57:32Z",
						UpdatedAt: "2023-01-19T14:57:32Z",
					},
					{
						GUID:      "3",
						Name:      "third-test-si",
						CreatedAt: "2023-01-16T14:57:32Z",
						UpdatedAt: "2023-01-20:57:32Z",
					},
				}, nil)
			})

			DescribeTable("ordering results", func(orderBy string, expectedOrder ...any) {
				req := createHttpRequest("GET", "/v3/service_instances?order_by="+orderBy, nil)
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.resources[*].guid", expectedOrder)))
			},
				Entry("created_at ASC", "created_at", "3", "2", "1"),
				Entry("created_at DESC", "-created_at", "1", "2", "3"),
				Entry("updated_at ASC", "updated_at", "1", "2", "3"),
				Entry("updated_at DESC", "-updated_at", "3", "2", "1"),
				Entry("name ASC", "name", "1", "2", "3"),
				Entry("name DESC", "-name", "3", "2", "1"),
			)

			When("order_by is not a valid field", func() {
				BeforeEach(func() {
					reqPath += "?order_by=foo"
				})

				It("returns an Unknown key error", func() {
					expectUnknownKeyError("The query parameter is invalid: Order by can only be: 'created_at', 'updated_at', 'name'")
				})
			})
		})

		When("the per_page query parameter is provided", func() {
			BeforeEach(func() {
				reqPath += "?per_page=10"
			})

			It("correctly sets the query parameter in response pagination links", func() {
				Expect(rr.Body.String()).To(ContainSubstring("/v3/service_instances?per_page=10"))
			})
		})

		When("there is an error fetching service instances", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				reqPath += "?foo=bar"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'names, space_guids, fields, order_by, per_page'")
			})
		})
	})

	Describe("PATCH /v3/service_instances/:guid", func() {
		BeforeEach(func() {
			payload = &payloads.ServiceInstancePatch{
				Name:        tools.PtrTo("new-name"),
				Tags:        &[]string{"alice", "bob"},
				Credentials: &map[string]string{"foo": "bar"},
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{"ann2": tools.PtrTo("ann_val2")},
					Labels:      map[string]*string{"lab2": tools.PtrTo("lab_val2")},
				},
			}

			serviceInstanceRepo.PatchServiceInstanceReturns(repositories.ServiceInstanceRecord{
				Name: "new-name",
				GUID: "service-instance-guid",
			}, nil)

			reqPath += "/service-instance-guid"
			reqMethod = http.MethodPatch
		})

		It("patches the service instance", func() {
			// TODO: fix me
			// Expect(payloadValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			// req, _ := payloadValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			// body, err := io.ReadAll(req.Body)
			// Expect(err).NotTo(HaveOccurred())
			// Expect(body).To(MatchJSON(`{"input": "json"}`))

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
				Credentials: &map[string]string{"foo": "bar"},
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
				payloadValidator.ValidatePayloadReturns(apierrors.NewUnprocessableEntityError(nil, "nope"))
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
			Expect(message.SpaceGUID).To(Equal("space-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
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

		When("getting the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				expectUnknownError()
			})
		})

		When("deleting the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				expectUnknownError()
			})
		})
	})
})
