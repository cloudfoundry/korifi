package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceInstance", func() {
	const (
		serviceInstanceGUID             = "test-service-instance-guid"
		serviceInstanceSpaceGUID        = "test-space-guid"
		serviceInstanceTypeUserProvided = "user-provided"
	)

	var (
		req                 *http.Request
		serviceInstanceRepo *fake.CFServiceInstanceRepository
		spaceRepo           *fake.SpaceRepository
	)

	BeforeEach(func() {
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		spaceRepo = new(fake.SpaceRepository)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewServiceInstance(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
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
				spaceRepo.GetSpaceReturns(
					repositories.SpaceRecord{},
					apierrors.NewNotFoundError(errors.New("not found"), repositories.SpaceResourceType),
				)

				makePostRequest(validBody)
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

				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("creating the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.CreateServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("space-instance-creation-failed"))
				makePostRequest(validBody)
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/service_instances", func() {
		const (
			serviceInstanceName1 = "my-upsi-1"
			serviceInstanceGUID1 = "service-instance-guid-1"

			serviceInstanceName2 = "my-upsi-2"
			serviceInstanceGUID2 = "service-instance-guid-2"
		)

		makeListRequest := func(queryParams ...string) {
			var err error
			listServiceInstanceUrl := "/v3/service_instances"
			if len(queryParams) > 0 {
				listServiceInstanceUrl += "?" + strings.Join(queryParams, "&")
			}
			req, err = http.NewRequestWithContext(ctx, "GET", listServiceInstanceUrl, nil)
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{
				{
					Name:       serviceInstanceName1,
					GUID:       serviceInstanceGUID1,
					SpaceGUID:  serviceInstanceSpaceGUID,
					SecretName: serviceInstanceGUID1,
					Tags:       []string{"foo", "bar"},
					Type:       serviceInstanceTypeUserProvided,
					CreatedAt:  "1906-04-18T13:12:00Z",
					UpdatedAt:  "1906-04-18T13:12:00Z",
				},
				{
					Name:       serviceInstanceName2,
					GUID:       serviceInstanceGUID2,
					SpaceGUID:  serviceInstanceSpaceGUID,
					SecretName: serviceInstanceGUID2,
					Tags:       nil,
					Type:       serviceInstanceTypeUserProvided,
					CreatedAt:  "1906-04-18T13:12:00Z",
					UpdatedAt:  "1906-04-18T13:12:01Z",
				},
			}, nil)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				makeListRequest()
			})

			It("invokes the repository with the provided auth info", func() {
				Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			When("no query parameters are provided", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Paginated Service Instance resources in the response", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
					Expect(rr.Body.String()).Should(MatchJSON(fmt.Sprintf(`{
					  "pagination": {
						"total_results": 2,
						"total_pages": 1,
						"first": {
						  "href": "%[1]s/v3/service_instances"
						},
						"last": {
						  "href": "%[1]s/v3/service_instances"
						},
						"next": null,
						"previous": null
					  },
					  "resources": [
						{
						  "guid": "%[3]s",
						  "created_at": "1906-04-18T13:12:00Z",
						  "updated_at": "1906-04-18T13:12:00Z",
						  "name": "%[2]s",
						  "tags": ["foo", "bar"],
						  "type": "%[5]s",
						  "syslog_drain_url": null,
						  "route_service_url": null,
						  "last_operation": {
							"type": "create",
							"state": "succeeded",
							"description": "Operation succeeded",
							"updated_at": "1906-04-18T13:12:00Z",
							"created_at": "1906-04-18T13:12:00Z"
						  },
						  "relationships": {
							"space": {
							  "data": {
							   "guid": "%[4]s"
							  }
							}
						  },
						  "metadata": {
							"labels": {},
							"annotations": {}
						  },
						  "links": {
							"self": {
							  "href": "%[1]s/v3/service_instances/%[3]s"
							},
							"space": {
							  "href": "%[1]s/v3/spaces/%[4]s"
							},
							"credentials": {
							  "href": "%[1]s/v3/service_instances/%[3]s/credentials"
							},
							"service_credential_bindings": {
							  "href": "%[1]s/v3/service_credential_bindings?service_instance_guids=%[3]s"
							},
							"service_route_bindings": {
							  "href": "%[1]s/v3/service_route_bindings?service_instance_guids=%[3]s"
							}
						  }
						},
						{
						  "guid": "%[7]s",
						  "created_at": "1906-04-18T13:12:00Z",
						  "updated_at": "1906-04-18T13:12:01Z",
						  "name": "%[6]s",
						  "tags": [],
						  "type": "%[5]s",
						  "syslog_drain_url": null,
						  "route_service_url": null,
						  "last_operation": {
							"type": "update",
							"state": "succeeded",
							"description": "Operation succeeded",
							"updated_at": "1906-04-18T13:12:01Z",
							"created_at": "1906-04-18T13:12:00Z"
						  },
						  "relationships": {
							"space": {
							  "data": {
							   "guid": "%[4]s"
							  }
							}
						  },
						  "metadata": {
							"labels": {},
							"annotations": {}
						  },
						  "links": {
							"self": {
							  "href": "%[1]s/v3/service_instances/%[7]s"
							},
							"space": {
							  "href": "%[1]s/v3/spaces/%[4]s"
							},
							"credentials": {
							  "href": "%[1]s/v3/service_instances/%[7]s/credentials"
							},
							"service_credential_bindings": {
							  "href": "%[1]s/v3/service_credential_bindings?service_instance_guids=%[7]s"
							},
							"service_route_bindings": {
							  "href": "%[1]s/v3/service_route_bindings?service_instance_guids=%[7]s"
							}
						  }
						}
					  ]
					}`, defaultServerURL, serviceInstanceName1, serviceInstanceGUID1, serviceInstanceSpaceGUID, serviceInstanceTypeUserProvided, serviceInstanceName2, serviceInstanceGUID2)))
				})
			})

			When("filtering query parameters are provided", func() {
				BeforeEach(func() {
					makeListRequest(
						"names=sc1,sc2",
						"space_guids=space1,space2",
						"fields%5Bservice_plan.service_offering.service_broker%5D=guid%2Cname",
					)
				})

				It("passes them to the repository", func() {
					Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
					_, _, message := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)

					Expect(message.Names).To(ConsistOf("sc1", "sc2"))
					Expect(message.SpaceGuids).To(ConsistOf("space1", "space2"))
				})

				It("correctly sets query parameters in response pagination links", func() {
					Expect(rr.Body.String()).To(ContainSubstring("/v3/service_instances?names=sc1,sc2&space_guids=space1,space2&fields%5Bservice_plan.service_offering.service_broker%5D=guid%2Cname"))
				})
			})

			When("the order_by query parameter is provided", func() {
				BeforeEach(func() {
					makeListRequest("order_by=-name")
				})

				It("correctly sets the query parameter in response pagination links", func() {
					Expect(rr.Body.String()).To(ContainSubstring("/v3/service_instances?order_by=-name"))
				})
			})

			Describe("Order results", func() {
				type res struct {
					GUID string `json:"guid"`
				}
				type resList struct {
					Resources []res `json:"resources"`
				}

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

				DescribeTable("ordering results", func(orderBy string, expectedOrder ...string) {
					req = createHttpRequest("GET", "/v3/service_instances?order_by="+orderBy, nil)
					rr = httptest.NewRecorder()
					routerBuilder.Build().ServeHTTP(rr, req)
					var respList resList
					err := json.Unmarshal(rr.Body.Bytes(), &respList)
					Expect(err).NotTo(HaveOccurred())
					expectedList := make([]res, len(expectedOrder))
					for i := range expectedOrder {
						expectedList[i] = res{GUID: expectedOrder[i]}
					}
					Expect(respList.Resources).To(Equal(expectedList))
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
						makeListRequest("order_by=foo")
					})

					It("returns an Unknown key error", func() {
						expectUnknownKeyError("The query parameter is invalid: Order by can only be: 'created_at', 'updated_at', 'name'")
					})
				})
			})

			When("the per_page query parameter is provided", func() {
				BeforeEach(func() {
					makeListRequest("per_page=10")
				})

				It("handles the request", func() {
					Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
				})

				It("correctly sets the query parameter in response pagination links", func() {
					Expect(rr.Body.String()).To(ContainSubstring("/v3/service_instances?per_page=10"))
				})
			})
		})

		When("no service instances can be found", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{}, nil)
				makeListRequest()
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
					"href": "%[1]s/v3/service_instances"
				  },
				  "last": {
					"href": "%[1]s/v3/service_instances"
				  },
				  "next": null,
				  "previous": null
				},
				"resources": []
			}`, defaultServerURL)), "Response body matches response:")
			})
		})

		When("there is an error fetching service instances", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{}, errors.New("unknown!"))
				makeListRequest()
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				makeListRequest("foo=bar")
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'names, space_guids, fields, order_by, per_page'")
			})
		})
	})

	Describe("the DELETE /v3/service_instances endpoint", func() {
		BeforeEach(func() {
			serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{SpaceGUID: spaceGUID}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("/v3/service_instances/%s", serviceInstanceGUID), nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 204 No Content", func() {
			Expect(rr.Code).To(Equal(http.StatusNoContent))
		})

		It("gets the service instance", func() {
			Expect(serviceInstanceRepo.GetServiceInstanceCallCount()).To(Equal(1))
			_, _, actualGUID := serviceInstanceRepo.GetServiceInstanceArgsForCall(0)
			Expect(actualGUID).To(Equal(serviceInstanceGUID))
		})

		It("deletes the service instance using the repo", func() {
			Expect(serviceInstanceRepo.DeleteServiceInstanceCallCount()).To(Equal(1))
			_, _, message := serviceInstanceRepo.DeleteServiceInstanceArgsForCall(0)
			Expect(message.GUID).To(Equal(serviceInstanceGUID))
			Expect(message.SpaceGUID).To(Equal(spaceGUID))
		})

		When("getting the service instance fails with forbidden", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(
					repositories.ServiceInstanceRecord{},
					apierrors.NewForbiddenError(nil, repositories.ServiceInstanceResourceType),
				)
			})

			It("returns 404 Not Found", func() {
				Expect(rr.Code).To(Equal(http.StatusNotFound))
			})
		})

		When("getting the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.GetServiceInstanceReturns(repositories.ServiceInstanceRecord{}, errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		When("deleting the service instance fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.DeleteServiceInstanceReturns(errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				Expect(rr.Code).To(Equal(http.StatusInternalServerError))
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
