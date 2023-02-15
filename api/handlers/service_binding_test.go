package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceBinding", func() {
	const (
		appGUID                = "test-app-guid"
		serviceBindingGUID     = "some-generated-guid"
		serviceInstanceGUID    = "test-service-instance-guid"
		spaceGUID              = "test-space-guid"
		listServiceBindingsUrl = "/v3/service_credential_bindings"
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
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/service_credential_bindings", func() {
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
			serviceBindingRepo.GetServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID:                serviceBindingGUID,
				Name:                tools.PtrTo("some-binding-name"),
				Type:                "app",
				AppGUID:             appGUID,
				ServiceInstanceGUID: serviceInstanceGUID,
				SpaceGUID:           spaceGUID,
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"bar": "baz",
				},
				CreatedAt: "created-on",
				UpdatedAt: "updated-on",
				LastOperation: repositories.ServiceBindingLastOperation{
					Type:        "op1",
					State:       "state1",
					Description: tools.PtrTo("desc"),
					CreatedAt:   "op1-created-on",
					UpdatedAt:   "op1-updated-on",
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		It("has the correct response type", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
		})

		It("returns the correct JSON", func() {
			Expect(rr.Body.String()).To(MatchJSON(`
				{
					"guid": "some-generated-guid",
					"name": "some-binding-name",
					"created_at": "created-on",
					"updated_at": "updated-on",
					"type": "app",
					"last_operation": {
					  "type": "op1",
					  "state": "state1",
					  "description": "desc",
					  "created_at": "op1-created-on",
					  "updated_at": "op1-updated-on"
					},
					"metadata": {
						"labels": {
							"foo": "bar"
						},
						"annotations": {
							"bar": "baz"
						}
					},
					"relationships": {
						"app": {
						  "data": {
							"guid": "test-app-guid"
						  }
						},
						"service_instance": {
						  "data": {
							"guid": "test-service-instance-guid"
						  }
						}
					},
					"links": {
						"self": {
						  "href": "https://api.example.org/v3/service_credential_bindings/some-generated-guid"
						},
						"details": {
						  "href": "https://api.example.org/v3/service_credential_bindings/some-generated-guid/details"
						},
						"service_instance": {
						  "href": "https://api.example.org/v3/service_instances/test-service-instance-guid"
						},
						"app": {
						  "href": "https://api.example.org/v3/apps/test-app-guid"
						}
				}
			}`))
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
				expectNotFoundError("CFServiceBinding not found")
			})
		})
	})

	Describe("GET /v3/service_credential_bindings", func() {
		BeforeEach(func() {
			serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{{
				GUID:                serviceBindingGUID,
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
			Expect(rr.Body.String()).To(ContainSubstring(serviceBindingGUID))
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
			When("a bogus service_instance_guid filter is provided", func() {
				BeforeEach(func() {
					req.URL.RawQuery = "include=app&service_instance_guids=1,2,3"
					serviceBindingRepo.ListServiceBindingsReturns([]repositories.ServiceBindingRecord{}, nil)
				})

				It("returns an empty response with no include block", func() {
					var responseJSON map[string]interface{}
					err := json.Unmarshal(rr.Body.Bytes(), &responseJSON)
					Expect(err).NotTo(HaveOccurred())
					Expect(responseJSON).NotTo(HaveKey("included"))
				})
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
				var responseJSON map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &responseJSON)
				Expect(err).NotTo(HaveOccurred())
				Expect(responseJSON).NotTo(HaveKey("included"))
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

	Describe("DELETE /v3/service_credential_bindings/:guid", func() {
		const serviceBindingGUID = "test-service-instance-guid"

		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "DELETE", "/v3/service_credential_bindings/"+serviceBindingGUID, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a NoContent status", func() {
			Expect(rr.Code).To(Equal(http.StatusNoContent))
			Expect(rr.Body.String()).To(BeEmpty())
		})

		It("invokes DeleteServiceBinding on the repository", func() {
			Expect(serviceBindingRepo.DeleteServiceBindingCallCount()).To(Equal(1))
			_, _, guid := serviceBindingRepo.DeleteServiceBindingArgsForCall(0)
			Expect(guid).To(Equal(serviceBindingGUID))
		})
	})

	Describe("PATCH /v3/service_credential_bindings/:guid", func() {
		BeforeEach(func() {
			serviceBindingRepo.GetServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID:                serviceBindingGUID,
				Type:                "app",
				AppGUID:             appGUID,
				ServiceInstanceGUID: serviceInstanceGUID,
				SpaceGUID:           spaceGUID,
			}, nil)

			serviceBindingRepo.UpdateServiceBindingReturns(repositories.ServiceBindingRecord{
				GUID:                serviceBindingGUID,
				Name:                tools.PtrTo("some-binding-name"),
				Type:                "app",
				AppGUID:             appGUID,
				ServiceInstanceGUID: serviceInstanceGUID,
				SpaceGUID:           spaceGUID,
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"bar": "baz",
				},
				CreatedAt: "created-on",
				UpdatedAt: "updated-on",
				LastOperation: repositories.ServiceBindingLastOperation{
					Type:        "op1",
					State:       "state1",
					Description: tools.PtrTo("desc"),
					CreatedAt:   "op1-created-on",
					UpdatedAt:   "op1-updated-on",
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(`{
				  "metadata": {
					"labels": {
					  "foo": "bar"
					},
					"annotations": {
					  "bar": "baz"
					}
				  }
				}`))
			Expect(err).NotTo(HaveOccurred())
		})

		It("has the correct response type", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
		})

		It("returns the correct JSON", func() {
			Expect(rr.Body.String()).To(MatchJSON(`
				{
					"guid": "some-generated-guid",
					"name": "some-binding-name",
					"created_at": "created-on",
					"updated_at": "updated-on",
					"type": "app",
					"last_operation": {
					  "type": "op1",
					  "state": "state1",
					  "description": "desc",
					  "created_at": "op1-created-on",
					  "updated_at": "op1-updated-on"
					},
					"metadata": {
						"labels": {
							"foo": "bar"
						},
						"annotations": {
							"bar": "baz"
						}
					},
					"relationships": {
						"app": {
						  "data": {
							"guid": "test-app-guid"
						  }
						},
						"service_instance": {
						  "data": {
							"guid": "test-service-instance-guid"
						  }
						}
					},
					"links": {
						"self": {
						  "href": "https://api.example.org/v3/service_credential_bindings/some-generated-guid"
						},
						"details": {
						  "href": "https://api.example.org/v3/service_credential_bindings/some-generated-guid/details"
						},
						"service_instance": {
						  "href": "https://api.example.org/v3/service_instances/test-service-instance-guid"
						},
						"app": {
						  "href": "https://api.example.org/v3/apps/test-app-guid"
						}
				}
			}`))
		})

		It("invokes the service binding repo correctly", func() {
			Expect(serviceBindingRepo.UpdateServiceBindingCallCount()).To(Equal(1))
			_, _, updateMessage := serviceBindingRepo.UpdateServiceBindingArgsForCall(0)
			Expect(updateMessage).To(Equal(repositories.UpdateServiceBindingMessage{
				GUID: serviceBindingGUID,
				MetadataPatch: repositories.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}))
		})

		When("the payload cannot be decoded", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(`{"one": "two"}`))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("invalid request body: json: unknown field \"one\"")
			})
		})

		When("a label is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(`{
					  "metadata": {
						"labels": {
						  "cloudfoundry.org/test": "production"
					    }
        		      }
					}`))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})

			When("the prefix is a subdomain of cloudfoundry.org", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(`{
					  "metadata": {
						"labels": {
						  "korifi.cloudfoundry.org/test": "production"
					    }
    		          }
					}`))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})
		})

		When("an annotation is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(`{
					  "metadata": {
						"annotations": {
						  "cloudfoundry.org/test": "there"
						}
					  }
					}`))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})

				When("the prefix is a subdomain of cloudfoundry.org", func() {
					BeforeEach(func() {
						var err error
						req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_credential_bindings/"+serviceBindingGUID, strings.NewReader(`{
						  "metadata": {
							"annotations": {
							  "korifi.cloudfoundry.org/test": "there"
							}
						  }
						}`))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns an unprocessable entity error", func() {
						expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
					})
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
				expectNotFoundError("CFServiceBinding not found")
			})
		})
	})
})
