package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("RouteHandler", func() {
	const (
		testDomainGUID = "test-domain-guid"
		testRouteGUID  = "test-route-guid"
		testRouteHost  = "test-route-host"
		testSpaceGUID  = "test-space-guid"
		testDomainName = "test-domain-name"
		testRoutePath  = "/test-route-path"
	)

	var (
		routeRepo  *fake.CFRouteRepository
		domainRepo *fake.CFDomainRepository
		appRepo    *fake.CFAppRepository
		spaceRepo  *fake.SpaceRepository

		requestMethod string
		requestPath   string
		requestBody   string
	)

	BeforeEach(func() {
		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		appRepo = new(fake.CFAppRepository)
		spaceRepo = new(fake.SpaceRepository)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		routeHandler := NewRouteHandler(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			decoderValidator,
		)
		routeHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	})

	Describe("the GET /v3/routes/:guid endpoint", func() {
		BeforeEach(func() {
			routeRepo.GetRouteReturns(repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				Domain: repositories.DomainRecord{
					GUID: testDomainGUID,
				},
				Host:      testRouteHost,
				Protocol:  "http",
				CreatedAt: "create-time",
				UpdatedAt: "update-time",
			}, nil)

			domainRepo.GetDomainReturns(repositories.DomainRecord{
				GUID: testDomainGUID,
				Name: "example.org",
			}, nil)

			requestMethod = http.MethodGet
			requestPath = fmt.Sprintf("/v3/routes/%s", testRouteGUID)
			requestBody = ""
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("provides the apierrors.Info from the context to the domain repository", func() {
				Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := domainRepo.GetDomainArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns the Route in the response", func() {
				expectedBody := fmt.Sprintf(`{
					"guid": "test-route-guid",
					"port": null,
					"path": "",
					"protocol": "http",
					"host": "test-route-host",
					"url": "test-route-host.example.org",
					"created_at": "create-time",
					"updated_at": "update-time",
					"destinations": [],
					"relationships": {
						"space": {
							"data": {
								"guid": "test-space-guid"
							}
						},
						"domain": {
							"data": {
								"guid": "test-domain-guid"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self":{
                            "href": "%[1]s/v3/routes/test-route-guid"
						},
						"space":{
                            "href": "%[1]s/v3/spaces/test-space-guid"
						},
						"domain":{
                            "href": "%[1]s/v3/domains/test-domain-guid"
						},
						"destinations":{
                            "href": "%[1]s/v3/routes/test-route-guid/destinations"
						}
					}
                }`, defaultServerURL)

				Expect(rr.Body.String()).To(MatchJSON(expectedBody), "Response body matches response:")
			})

			It("fetches the correct route", func() {
				Expect(routeRepo.GetRouteCallCount()).To(Equal(1), "Repo GetRoute was not called")
				_, _, actualRouteGUID := routeRepo.GetRouteArgsForCall(0)
				Expect(actualRouteGUID).To(Equal(testRouteGUID), "GetRoute was not passed the correct GUID")
			})

			It("fetches the correct domain", func() {
				Expect(domainRepo.GetDomainCallCount()).To(Equal(1), "Repo GetDomain was not called")
				_, _, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
				Expect(actualDomainGUID).To(Equal(testDomainGUID), "GetDomain was not passed the correct GUID")
			})
		})

		When("the route is not accessible", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Route not found")
			})
		})

		When("the route's domain is not accessible", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, apierrors.NewForbiddenError(nil, repositories.DomainResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Domain")
			})
		})

		When("there is some other error fetching the route", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is some other error fetching the domain", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/routes endpoint", func() {
		var (
			domainRecord repositories.DomainRecord
			routeRecord  repositories.RouteRecord
		)

		BeforeEach(func() {
			routeRecord = repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				Domain: repositories.DomainRecord{
					GUID: testDomainGUID,
				},
				Host:         testRouteHost,
				Path:         "/some_path",
				Protocol:     "http",
				Destinations: nil,
				Labels:       nil,
				Annotations:  nil,
				CreatedAt:    "2019-05-10T17:17:48Z",
				UpdatedAt:    "2019-05-10T17:17:48Z",
			}
			routeRepo.ListRoutesReturns([]repositories.RouteRecord{
				routeRecord,
			}, nil)

			domainRecord = repositories.DomainRecord{
				GUID: testDomainGUID,
				Name: "example.org",
			}
			domainRepo.GetDomainReturns(domainRecord, nil)

			requestMethod = http.MethodGet
			requestPath = "/v3/routes"
			requestBody = ""
		})

		When("on the happy path", func() {
			It("provides the apierrors.Info from the context to the domain repository", func() {
				Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := domainRepo.GetDomainArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("provides the apierrors.Info from the context to the routes repository", func() {
				Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := routeRepo.ListRoutesArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			When("query parameters are not provided", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the Pagination Data and App Resources in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
	   				"pagination": {
	   					"total_results": 1,
	   					"total_pages": 1,
	   					"first": {
	   						"href": "%[1]s/v3/routes"
	   					},
	   					"last": {
	   						"href": "%[1]s/v3/routes"
	   					},
	   					"next": null,
	   					"previous": null
	   				},
	   				"resources": [
	   					{
	   						"guid": "%[2]s",
	   						"port": null,
	   						"path": "%[3]s",
	   						"protocol": "%[4]s",
	   						"host": "%[5]s",
	   						"url": "%[5]s.%[6]s%[3]s",
	   						"created_at": "%[7]s",
	   						"updated_at": "%[8]s",
	   						"destinations": [],
	   						"relationships": {
	   							"space": {
	   								"data": {
	   									"guid": "%[9]s"
	   								}
	   							},
	   							"domain": {
	   								"data": {
	   									"guid": "%[10]s"
	   								}
	   							}
	   						},
	   						"metadata": {
	   							"labels": {},
	   							"annotations": {}
	   						},
	   						"links": {
	   							"self":{
	   								"href": "%[1]s/v3/routes/%[2]s"
	   							},
	   							"space":{
	   								"href": "%[1]s/v3/spaces/%[9]s"
	   							},
	   							"domain":{
	   								"href": "%[1]s/v3/domains/%[10]s"
	   							},
	   							"destinations":{
	   								"href": "%[1]s/v3/routes/%[2]s/destinations"
	   							}
	   						}
	   					}
	   				]
	   				}`, defaultServerURL, routeRecord.GUID, routeRecord.Path, routeRecord.Protocol, routeRecord.Host, domainRecord.Name, routeRecord.CreatedAt, routeRecord.UpdatedAt, routeRecord.SpaceGUID, domainRecord.GUID)), "Response body matches response:")
				})
			})

			When("app_guids query parameters are provided", func() {
				BeforeEach(func() {
					requestPath += "?app_guids=my-app-guid"
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Pagination Data with the app_guids filter", func() {
					Expect(rr.Body.String()).To(ContainSubstring("https://api.example.org/v3/routes?app_guids=my-app-guid"))
				})

				It("calls route with expected parameters", func() {
					Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
					_, _, message := routeRepo.ListRoutesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(1))
					Expect(message.AppGUIDs[0]).To(Equal("my-app-guid"))
				})
			})

			When("space_guids query parameters are provided", func() {
				BeforeEach(func() {
					requestPath += "?space_guids=my-space-guid"
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Pagination Data with the space_guids filter", func() {
					Expect(rr.Body.String()).To(ContainSubstring("https://api.example.org/v3/routes?space_guids=my-space-guid"))
				})

				It("calls route with expected parameters", func() {
					Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
					_, _, message := routeRepo.ListRoutesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(0))
					Expect(message.SpaceGUIDs).To(HaveLen(1))
					Expect(message.SpaceGUIDs[0]).To(Equal("my-space-guid"))
				})
			})

			When("domain_guids query parameters are provided", func() {
				BeforeEach(func() {
					requestPath += "?domain_guids=my-domain-guid"
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Pagination Data with the domain_guids filter", func() {
					Expect(rr.Body.String()).To(ContainSubstring("https://api.example.org/v3/routes?domain_guids=my-domain-guid"))
				})

				It("calls route with expected parameters", func() {
					Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
					_, _, message := routeRepo.ListRoutesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(0))
					Expect(message.DomainGUIDs).To(HaveLen(1))
					Expect(message.DomainGUIDs[0]).To(Equal("my-domain-guid"))
				})
			})

			When("hosts query parameters are provided", func() {
				BeforeEach(func() {
					requestPath += "?hosts=my-host"
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Pagination Data with the hosts filter", func() {
					Expect(rr.Body.String()).To(ContainSubstring("https://api.example.org/v3/routes?hosts=my-host"))
				})

				It("calls route with expected parameters", func() {
					Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
					_, _, message := routeRepo.ListRoutesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(0))
					Expect(message.Hosts).To(HaveLen(1))
					Expect(message.Hosts[0]).To(Equal("my-host"))
				})
			})

			When("hosts query parameter is provided with no value", func() {
				BeforeEach(func() {
					requestPath += "?hosts="
					routeRepo.ListRoutesReturns([]repositories.RouteRecord{}, nil)
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Pagination Data with the hosts filter", func() {
					response := map[string]interface{}{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response).To(SatisfyAll(
						HaveKeyWithValue("pagination", HaveKeyWithValue("first", HaveKeyWithValue("href", "https://api.example.org/v3/routes?hosts="))),
						HaveKeyWithValue("resources", BeEmpty()),
					))
				})

				It("calls route with expected parameters", func() {
					Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
					_, _, message := routeRepo.ListRoutesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(0))
					Expect(message.Hosts).To(HaveLen(1))
					Expect(message.Hosts[0]).To(Equal(""))
				})
			})

			When("paths query parameters are provided", func() {
				BeforeEach(func() {
					requestPath += "?paths=/some/path"
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Pagination Data with the paths filter", func() {
					Expect(rr.Body.String()).To(ContainSubstring("https://api.example.org/v3/routes?paths=/some/path"))
				})

				It("calls route with expected parameters", func() {
					Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
					_, _, message := routeRepo.ListRoutesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(0))
					Expect(message.Paths).To(HaveLen(1))
					Expect(message.Paths[0]).To(Equal("/some/path"))
				})
			})
		})

		When("no routes exist", func() {
			BeforeEach(func() {
				routeRepo.ListRoutesReturns([]repositories.RouteRecord{}, nil)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns an empty list in the response", func() {
				expectedBody := fmt.Sprintf(`{
	   					"pagination": {
	   						"total_results": 0,
	   						"total_pages": 1,
	   						"first": {
	   							"href": "%[1]s/v3/routes"
	   						},
	   						"last": {
	   							"href": "%[1]s/v3/routes"
	   						},
	   						"next": null,
	   						"previous": null
	   					},
	   					"resources": [
	   					]
	   				}`, defaultServerURL)

				Expect(rr.Body.String()).To(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		When("there is a failure Listing Routes", func() {
			BeforeEach(func() {
				routeRepo.ListRoutesReturns([]repositories.RouteRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is a failure finding a Domain", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				requestPath += "?foo=my-app-guid"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'app_guids, space_guids, domain_guids, hosts, paths'")
			})
		})
	})

	Describe("the POST /v3/routes endpoint", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/routes"

			spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
				Name: testSpaceGUID,
			}, nil)

			domainRepo.GetDomainReturns(repositories.DomainRecord{
				GUID: testDomainGUID,
				Name: testDomainName,
			}, nil)

			routeRepo.CreateRouteReturns(repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				Domain: repositories.DomainRecord{
					GUID: testDomainGUID,
				},
				Host:      testRouteHost,
				Path:      testRoutePath,
				Protocol:  "http",
				CreatedAt: "create-time",
				UpdatedAt: "update-time",
			}, nil)

			requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, nil)
		})

		When("the space exists and the route does not exist and", func() {
			When("a plain POST test route request is sent without metadata", func() {
				It("checks that the specified namespace exists", func() {
					Expect(spaceRepo.GetSpaceCallCount()).To(Equal(1))
					_, _, actualSpaceGUID := spaceRepo.GetSpaceArgsForCall(0)
					Expect(actualSpaceGUID).To(Equal(testSpaceGUID))
				})

				It("checks that the specified domain exists", func() {
					Expect(domainRepo.GetDomainCallCount()).To(Equal(1), "Repo GetDomain was not called")
					_, _, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
					Expect(actualDomainGUID).To(Equal(testDomainGUID), "GetDomain was not passed the correct GUID")
				})

				It("provides the apierrors.Info from the context to the domain repository", func() {
					Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
					_, actualAuthInfo, _ := domainRepo.GetDomainArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
				})

				It("provides the apierrors.Info from the context to the routes repository", func() {
					Expect(routeRepo.CreateRouteCallCount()).To(Equal(1))
					_, actualAuthInfo, _ := routeRepo.CreateRouteArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
				})

				It("returns status 201 Created", func() {
					Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the created route in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
	   						"guid": "test-route-guid",
	   						"protocol": "http",
	   						"port": null,
	   						"host": "test-route-host",
	   						"path": "/test-route-path",
	   						"url": "test-route-host.test-domain-name/test-route-path",
	   						"created_at": "create-time",
	   						"updated_at": "update-time",
	   						"destinations": [],
	   						"metadata": {
	   							"labels": {},
	   							"annotations": {}
	   						},
	   						"relationships": {
	   							"space": {
	   								"data": {
	   									"guid": "test-space-guid"
	   								}
	   							},
	   							"domain": {
	   								"data": {
	   									"guid": "test-domain-guid"
	   								}
	   							}
	   						},
	   						"links": {
	   							"self": {
	                                   "href": "%[1]s/v3/routes/test-route-guid"
	   							},
	   							"space": {
	                                   "href": "%[1]s/v3/spaces/test-space-guid"
	   							},
	   							"domain": {
	                                   "href": "%[1]s/v3/domains/test-domain-guid"
	   							},
	   							"destinations": {
	                                   "href": "%[1]s/v3/routes/test-route-guid/destinations"
	   							}
	   						}
	                       }`, defaultServerURL)), "Response body mismatch")
				})
			})

			When("a POST test route request is sent with metadata labels", func() {
				var testLabels map[string]string

				BeforeEach(func() {
					testLabels = map[string]string{"label1": "foo", "label2": "bar"}

					requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, testLabels, nil)
				})

				It("should pass along the labels to CreateRoute", func() {
					Expect(routeRepo.CreateRouteCallCount()).To(Equal(1), "Repo CreateRoute count was not invoked 1 time")
					_, _, createRouteRecord := routeRepo.CreateRouteArgsForCall(0)
					Expect(createRouteRecord.Labels).To(Equal(testLabels))
				})
			})

			When("a POST test route request is sent with metadata annotations", func() {
				var testAnnotations map[string]string

				BeforeEach(func() {
					testAnnotations = map[string]string{"annotation1": "foo", "annotation2": "bar"}
					requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, testAnnotations)
				})

				It("should pass along the annotations to CreateRoute", func() {
					Expect(routeRepo.CreateRouteCallCount()).To(Equal(1), "Repo CreateRoute count was not invoked 1 time")
					_, _, createRouteRecord := routeRepo.CreateRouteArgsForCall(0)
					Expect(createRouteRecord.Annotations).To(Equal(testAnnotations))
				})
			})
		})

		When("the request body is invalid JSON", func() {
			BeforeEach(func() {
				requestBody = `{`
			})

			It("returns a status 400 Bad Request ", func() {
				Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
	   					"errors": [
	   						{
	   							"title": "CF-MessageParseError",
	   							"detail": "Request invalid due to parse error: invalid request body",
	   							"code": 1001
	   						}
	   					]
	   				}`), "Response body matches response:")
			})
		})

		When("the request body includes an unknown description field", func() {
			BeforeEach(func() {
				requestBody = `{"description" : "Invalid Request"}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "description"`)
			})
		})

		When("the host is not a string", func() {
			BeforeEach(func() {
				requestBody = `{
	   					"host": 12345,
	   					"relationships": {
	   						"space": {
	   							"data": {
	   								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
	   							}
	   						}
	   					}
	   				}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Host must be a string")
			})
		})

		When("the request body is missing the domain relationship", func() {
			BeforeEach(func() {
				requestBody = `{
	   					"host": "test-route-host",
	   					"relationships": {
	   						"space": {
	   							"data": {
	   								"guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
	   							}
	   						}
	   					}
	   				}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Data is a required field")
			})
		})

		When("the request body is missing the space relationship", func() {
			BeforeEach(func() {
				requestBody = `{
	   					"host": "test-route-host",
	   					"relationships": {
	   						"domain": {
	   							"data": {
	   								"guid": "0b78dd5d-c723-4f2e-b168-df3c3e1d0806"
	   							}
	   						}
	   					}
	   				}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Data is a required field")
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{},
					apierrors.NewNotFoundError(errors.New("not found"), repositories.SpaceResourceType))

				requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("the space is forbidden", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{},
					apierrors.NewForbiddenError(errors.New("not found"), repositories.SpaceResourceType))

				requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("GetSpace returns an unknown error", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{},
					errors.New("random error"))

				requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, apierrors.NewNotFoundError(nil, repositories.DomainResourceType))
				requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid domain. Ensure that the domain exists and you have access to it.")
			})
		})

		When("GetDomain returns an unknown error", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, errors.New("random error"))
				requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("CreateRoute returns an unknown error", func() {
			BeforeEach(func() {
				routeRepo.CreateRouteReturns(repositories.RouteRecord{},
					errors.New("random error"))

				requestBody = initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/routes/:guid endpoint", func() {
		BeforeEach(func() {
			requestMethod = "PATCH"
			requestPath = "/v3/routes/" + testRouteGUID
		})

		When("the route exists and is accessible and we patch the annotations and labels", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{
					GUID:      testRouteGUID,
					SpaceGUID: spaceGUID,
				}, nil)
				routeRepo.PatchRouteMetadataReturns(repositories.RouteRecord{
					GUID: testRouteGUID,
					Labels: map[string]string{
						"env":                           "production",
						"foo.example.com/my-identifier": "aruba",
					},
					Annotations: map[string]string{
						"hello":                       "there",
						"foo.example.com/lorem-ipsum": "Lorem ipsum.",
					},
					SpaceGUID: spaceGUID,
				}, nil)
				requestBody = `{
				  "metadata": {
					"labels": {
					  "env": "production",
					  "foo.example.com/my-identifier": "aruba"
					},
					"annotations": {
					  "hello": "there",
					  "foo.example.com/lorem-ipsum": "Lorem ipsum."
					}
				  }
				}`
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("patches the route with the new labels and annotations", func() {
				Expect(routeRepo.PatchRouteMetadataCallCount()).To(Equal(1))
				_, _, msg := routeRepo.PatchRouteMetadataArgsForCall(0)
				Expect(msg.RouteGUID).To(Equal(testRouteGUID))
				Expect(msg.SpaceGUID).To(Equal(spaceGUID))
				Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
				Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
				Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
				Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))
			})

			It("includes the labels and annotations in the response", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				var jsonBody struct {
					Metadata struct {
						Annotations map[string]string `json:"annotations"`
						Labels      map[string]string `json:"labels"`
					} `json:"metadata"`
				}
				Expect(json.NewDecoder(rr.Body).Decode(&jsonBody)).To(Succeed())
				Expect(jsonBody.Metadata.Annotations).To(Equal(map[string]string{
					"hello":                       "there",
					"foo.example.com/lorem-ipsum": "Lorem ipsum.",
				}))
				Expect(jsonBody.Metadata.Labels).To(Equal(map[string]string{
					"env":                           "production",
					"foo.example.com/my-identifier": "aruba",
				}))
			})
		})

		When("the user doesn't have permission to get the Route", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
				requestBody = `{
					  "metadata": {
						"labels": {
						  "env": "production"
						}
					  }
					}`
			})

			It("returns a not found error", func() {
				expectNotFoundError("Route not found")
			})

			It("does not call patch", func() {
				Expect(routeRepo.PatchRouteMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the Route errors", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
				requestBody = `{
					  "metadata": {
						"labels": {
						  "env": "production",
						}
					  }
					}`
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("does not call patch", func() {
				Expect(routeRepo.PatchRouteMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the Route errors", func() {
			BeforeEach(func() {
				routeRecord := repositories.RouteRecord{
					GUID:      testRouteGUID,
					SpaceGUID: spaceGUID,
				}
				routeRepo.GetRouteReturns(routeRecord, nil)
				routeRepo.PatchRouteMetadataReturns(repositories.RouteRecord{}, errors.New("boom"))
				requestBody = `{
					  "metadata": {
						"labels": {
						  "env": "production"
						}
					  }
					}`
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("a label is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					requestBody = `{
						  "metadata": {
							"labels": {
							  "cloudfoundry.org/test": "production"
						    }
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
							"labels": {
							  "korifi.cloudfoundry.org/test": "production"
						    }
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
							"annotations": {
							  "cloudfoundry.org/test": "there"
							}
						  }
						}`
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})

				When("the prefix is a subdomain of cloudfoundry.org", func() {
					BeforeEach(func() {
						requestBody = `{
						  "metadata": {
							"annotations": {
							  "korifi.cloudfoundry.org/test": "there"
							}
						  }
						}`
					})

					It("returns an unprocessable entity error", func() {
						expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
					})
				})
			})
		})
	})

	Describe("the GET /v3/routes/:guid/destinations endpoint", func() {
		var routeRecord repositories.RouteRecord

		BeforeEach(func() {
			routeRecord = repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				Domain: repositories.DomainRecord{
					GUID: testDomainGUID,
				},
				Host:     testRouteHost,
				Protocol: "http",
				Destinations: []repositories.DestinationRecord{
					{
						GUID:        "89323d4e-2e84-43e7-83e9-adbf50a20c0e",
						AppGUID:     "1cb006ee-fb05-47e1-b541-c34179ddc446",
						ProcessType: "web",
						Port:        8080,
						Protocol:    "http1",
					},
					{
						GUID:        "fbef10a2-8ee7-11e9-aa2d-abeeaf7b83c5",
						AppGUID:     "01856e12-8ee8-11e9-98a5-bb397dbc818f",
						ProcessType: "api",
						Port:        9000,
						Protocol:    "http1",
					},
				},
				CreatedAt: "create-time",
				UpdatedAt: "update-time",
			}
			routeRepo.GetRouteReturns(routeRecord, nil)

			requestMethod = http.MethodGet
			requestPath = fmt.Sprintf("/v3/routes/%s/destinations", testRouteGUID)
			requestBody = ""
		})

		When("On the happy path and", func() {
			When("the Route has destinations", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("provides the apierrors.Info from the context to the routes repository", func() {
					Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
					_, actualAuthInfo, _ := routeRepo.GetRouteArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the Destinations in the response", func() {
					expectedBody := fmt.Sprintf(`{
							"destinations": [
								{
									"guid": "%[3]s",
									"app": {
										"guid": "%[4]s",
										"process": {
											"type": "%[5]s"
										}
									},
									"weight": null,
									"port": %[6]d,
									"protocol": "http1"
								},
								{
									"guid": "%[7]s",
									"app": {
										"guid": "%[8]s",
										"process": {
											"type": "%[9]s"
										}
									},
									"weight": null,
									"port": %[10]d,
									"protocol": "http1"
								}
							],
							"links": {
								"self": {
									"href": "%[1]s/v3/routes/%[2]s/destinations"
								},
								"route": {
									"href": "%[1]s/v3/routes/%[2]s"
								}
							}
						}`, defaultServerURL, testRouteGUID,
						routeRecord.Destinations[0].GUID, routeRecord.Destinations[0].AppGUID, routeRecord.Destinations[0].ProcessType, routeRecord.Destinations[0].Port,
						routeRecord.Destinations[1].GUID, routeRecord.Destinations[1].AppGUID, routeRecord.Destinations[1].ProcessType, routeRecord.Destinations[1].Port)

					Expect(rr.Body.String()).To(MatchJSON(expectedBody), "Response body matches response:")
				})
			})

			When("the Route has no destinations", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(
						repositories.RouteRecord{
							GUID:      testRouteGUID,
							SpaceGUID: testSpaceGUID,
							Domain: repositories.DomainRecord{
								GUID: testDomainGUID,
							},
							Host:         testRouteHost,
							Protocol:     "http",
							Destinations: []repositories.DestinationRecord{},
							CreatedAt:    "create-time",
							UpdatedAt:    "update-time",
						}, nil)
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns no Destinations in the response", func() {
					expectedBody := fmt.Sprintf(`{
							"destinations": [],
							"links": {
								"self": {
									"href": "%[1]s/v3/routes/%[2]s/destinations"
								},
								"route": {
									"href": "%[1]s/v3/routes/%[2]s"
								}
							}
						}`, defaultServerURL, testRouteGUID)

					Expect(rr.Body.String()).To(MatchJSON(expectedBody), "Response body matches response:")
				})
			})
		})

		When("the route is not accessible", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Route not found")
			})
		})

		When("there is some other issue fetching the route", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the domain is not accessible", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Route not found")
			})
		})

		When("there is some other issue fetching the domain", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/routes/:guid/destinations endpoint", func() {
		const (
			routeGUID               = "test-route-guid"
			domainGUID              = "test-domain-guid"
			spaceGUID               = "test-space-guid"
			routeHost               = "test-app"
			destination1AppGUID     = "1cb006ee-fb05-47e1-b541-c34179ddc446"
			destination2AppGUID     = "01856e12-8ee8-11e9-98a5-bb397dbc818f"
			destination2ProcessType = "api"
			destination2Port        = 9000
			destination1GUID        = "destination1-guid"
			destination2GUID        = "destination2-guid"
		)

		var (
			domain      repositories.DomainRecord
			routeRecord repositories.RouteRecord
		)

		BeforeEach(func() {
			routeRecord = repositories.RouteRecord{
				GUID:         routeGUID,
				SpaceGUID:    spaceGUID,
				Domain:       repositories.DomainRecord{GUID: domainGUID},
				Host:         routeHost,
				Path:         "",
				Protocol:     "http",
				Destinations: nil,
			}

			domain = repositories.DomainRecord{
				GUID: domainGUID,
				Name: "my-tld.com",
			}

			routeRepo.GetRouteReturns(routeRecord, nil)
			domainRepo.GetDomainReturns(domain, nil)

			updatedRoute := routeRecord
			updatedRoute.Domain = domain
			updatedRoute.Destinations = []repositories.DestinationRecord{
				{
					GUID:        destination1GUID,
					AppGUID:     destination1AppGUID,
					ProcessType: "web",
					Port:        8080,
					Protocol:    "http1",
				},
				{
					GUID:        destination2GUID,
					AppGUID:     destination2AppGUID,
					ProcessType: destination2ProcessType,
					Port:        destination2Port,
					Protocol:    "http1",
				},
			}
			routeRepo.AddDestinationsToRouteReturns(updatedRoute, nil)

			requestMethod = http.MethodPost
			requestPath = "/v3/routes/" + routeGUID + "/destinations"
			requestBody = fmt.Sprintf(`{
						"destinations": [
							{
								"app": {
									"guid": %q
								},
								"protocol": "http1"
							},
							{
								"app": {
									"guid": %q,
									"process": {
										"type": %q
									}
								},
								"port": %d,
								"protocol": "http1"
							}
						]
					}`, destination1AppGUID, destination2AppGUID, destination2ProcessType, destination2Port)
		})

		When("the request body is valid", func() {
			It("passes the authInfo into the repo calls", func() {
				Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := routeRepo.GetRouteArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
				_, actualAuthInfo, _ = domainRepo.GetDomainArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
				_, actualAuthInfo, _ = routeRepo.AddDestinationsToRouteArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a success and a valid response", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")

				var parsedBody map[string]interface{}
				Expect(
					json.Unmarshal(rr.Body.Bytes(), &parsedBody),
				).To(Succeed())

				Expect(parsedBody["destinations"]).To(HaveLen(2))
				Expect(parsedBody["destinations"]).To(Equal([]interface{}{
					map[string]interface{}{
						"guid": destination1GUID,
						"app": map[string]interface{}{
							"guid": destination1AppGUID,
							"process": map[string]interface{}{
								"type": "web",
							},
						},
						"weight":   nil,
						"port":     float64(8080),
						"protocol": "http1",
					},
					map[string]interface{}{
						"guid": destination2GUID,
						"app": map[string]interface{}{
							"guid": destination2AppGUID,
							"process": map[string]interface{}{
								"type": destination2ProcessType,
							},
						},
						"weight":   nil,
						"port":     float64(destination2Port),
						"protocol": "http1",
					},
				}))

				Expect(parsedBody["links"]).To(Equal(map[string]interface{}{
					"self": map[string]interface{}{
						"href": "https://api.example.org/v3/routes/test-route-guid/destinations",
					},
					"route": map[string]interface{}{
						"href": "https://api.example.org/v3/routes/test-route-guid",
					},
				}))
			})

			It("adds the new destinations to the Route", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
				_, _, message := routeRepo.AddDestinationsToRouteArgsForCall(0)
				Expect(message.RouteGUID).To(Equal(routeGUID))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))

				Expect(message.NewDestinations).To(ConsistOf(
					MatchAllFields(Fields{
						"AppGUID":     Equal(destination1AppGUID),
						"ProcessType": Equal("web"),
						"Port":        Equal(8080),
						"Protocol":    Equal("http1"),
					}),
					MatchAllFields(Fields{
						"AppGUID":     Equal(destination2AppGUID),
						"ProcessType": Equal(destination2ProcessType),
						"Port":        Equal(destination2Port),
						"Protocol":    Equal("http1"),
					}),
				))
			})

			When("the route doesn't exist", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewNotFoundError(nil, repositories.RouteResourceType))
				})

				It("responds with 404 and an error", func() {
					expectNotFoundError("Route not found")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("the user lacks permission to fetch the route", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
				})

				It("responds with 404 and an error", func() {
					expectNotFoundError("Route not found")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("the destination protocol is not provided", func() {
				BeforeEach(func() {
					requestBody = fmt.Sprintf(`{
						"destinations": [
							{
								"app": {
									"guid": %q
								}
							},
							{
								"app": {
									"guid": %q,
									"process": {
										"type": %q
									}
								},
								"port": %d,
								"protocol": "http1"
							}
						]
					}`, destination1AppGUID, destination2AppGUID, destination2ProcessType, destination2Port)
				})
				It("defaults the protocol to `http1`", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")

					var parsedBody map[string]interface{}
					Expect(
						json.Unmarshal(rr.Body.Bytes(), &parsedBody),
					).To(Succeed())

					var destination map[string]interface{}

					Expect(parsedBody["destinations"]).To(HaveLen(2))
					destination = parsedBody["destinations"].([]interface{})[0].(map[string]interface{})
					Expect(destination["protocol"]).To(Equal("http1"))

					destination = parsedBody["destinations"].([]interface{})[1].(map[string]interface{})
					Expect(destination["protocol"]).To(Equal("http1"))
				})
			})

			When("fetching the route errors", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
				})

				It("responds with an Unknown Error", func() {
					expectUnknownError()
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("adding the destinations to the Route errors", func() {
				BeforeEach(func() {
					routeRepo.AddDestinationsToRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
				})

				It("responds with an Unknown Error", func() {
					expectUnknownError()
				})
			})

			When("adding a destination fails", func() {
				BeforeEach(func() {
					routeRepo.AddDestinationsToRouteReturns(repositories.RouteRecord{}, errors.New("failed"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
		})

		When("the request body is invalid", func() {
			When("JSON is invalid", func() {
				BeforeEach(func() {
					requestBody = `{ this_is_a_invalid_json }`
				})

				It("returns a status 400 Bad Request ", func() {
					Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
				})

				It("has the expected error response body", func() {
					Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).To(MatchJSON(`{
							"errors": [
								{
									"title": "CF-MessageParseError",
									"detail": "Request invalid due to parse error: invalid request body",
									"code": 1001
								}
							]
						}`), "Response body matches response:")
				})
			})

			When("app is missing", func() {
				BeforeEach(func() {
					requestBody = `{
							"destinations": [
							  {
								"port": 9000,
								"protocol": "http1"
							  }
							]
						}`
				})

				It("returns a status 422 Unprocessable Entity ", func() {
					expectUnprocessableEntityError("App is a required field")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("app GUID is missing", func() {
				BeforeEach(func() {
					requestBody = `{
							"destinations": [
							  {
								"app": {},
								"port": 9000,
								"protocol": "http1"
							  }
							]
						}`
				})

				It("returns a status 422 Unprocessable Entity ", func() {
					expectUnprocessableEntityError("GUID is a required field")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("process type is missing", func() {
				BeforeEach(func() {
					requestBody = `{
							"destinations": [
								{
									"app": {
										"guid": "01856e12-8ee8-11e9-98a5-bb397dbc818f",
										"process": {}
									},
									"port": 9000,
									"protocol": "http1"
								}
							]
						}`
				})

				It("returns a status 422 Unprocessable Entity ", func() {
					expectUnprocessableEntityError("Type is a required field")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("destination protocol is not http1", func() {
				BeforeEach(func() {
					requestBody = `{
							"destinations": [
							  {
								"app": {
								  "guid": "01856e12-8ee8-11e9-98a5-bb397dbc818f"
								},
								"port": 9000,
								"protocol": "http"
							  }
							]
						}`
				})

				It("returns a status 422 Unprocessable Entity ", func() {
					expectUnprocessableEntityError("Protocol must be one of [http1]")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})
		})
	})

	Describe("the DELETE /v3/routes/:guid/destinations/:destination_guid endpoint", func() {
		const (
			routeGuid       = "test-route-guid"
			domainGuid      = "test-domain-guid"
			spaceGuid       = "test-space-guid"
			routeHost       = "test-app"
			appGuid         = "1cb006ee-fb05-47e1-b541-c34179ddc446"
			destinationGuid = "destination-guid"
		)

		var (
			domain      repositories.DomainRecord
			routeRecord repositories.RouteRecord
		)

		BeforeEach(func() {
			routeRecord = repositories.RouteRecord{
				GUID:      routeGuid,
				SpaceGUID: spaceGuid,
				Domain:    repositories.DomainRecord{GUID: domainGuid},
				Host:      routeHost,
				Path:      "",
				Protocol:  "http",
				Destinations: []repositories.DestinationRecord{
					{
						GUID:        destinationGuid,
						AppGUID:     appGuid,
						ProcessType: "web",
						Port:        8080,
						Protocol:    "http1",
					},
				},
			}

			domain = repositories.DomainRecord{
				GUID: domainGuid,
				Name: "my-tld.com",
			}

			routeRepo.GetRouteReturns(routeRecord, nil)
			domainRepo.GetDomainReturns(domain, nil)

			requestMethod = http.MethodDelete
			requestPath = "/v3/routes/" + routeGuid + "/destinations/" + destinationGuid
			requestBody = ""
		})

		When("the request is valid", func() {
			It("passes the authInfo into the repo calls", func() {
				Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := routeRepo.GetRouteArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
				_, actualAuthInfo, _ = domainRepo.GetDomainArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(1))
				_, actualAuthInfo, _ = routeRepo.RemoveDestinationFromRouteArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a success and a valid response", func() {
				Expect(rr.Code).To(Equal(http.StatusNoContent), "Matching HTTP response code:")

				Expect(rr.Body.String()).To(BeEmpty())
			})

			It("removes the destination from the Route", func() {
				Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(1))
				_, _, message := routeRepo.RemoveDestinationFromRouteArgsForCall(0)
				Expect(message.RouteGUID).To(Equal(routeGuid))
				Expect(message.SpaceGUID).To(Equal(spaceGuid))
				Expect(message.DestinationGuid).To(Equal(destinationGuid))

				Expect(message.ExistingDestinations).To(ConsistOf(
					MatchAllFields(Fields{
						"GUID":        Equal(destinationGuid),
						"AppGUID":     Equal(appGuid),
						"ProcessType": Equal("web"),
						"Port":        Equal(8080),
						"Protocol":    Equal("http1"),
					}),
				))
			})

			When("the route doesn't exist", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewNotFoundError(nil, repositories.RouteResourceType))
				})

				It("responds with 404 and an error", func() {
					expectNotFoundError("Route not found")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("the user lacks permission to fetch the route", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
				})

				It("responds with 404 and an error", func() {
					expectNotFoundError("Route not found")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("fetching the route errors", func() {
				BeforeEach(func() {
					routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
				})

				It("responds with an Unknown Error", func() {
					expectUnknownError()
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("removing the destinations from the Route errors", func() {
				BeforeEach(func() {
					routeRepo.RemoveDestinationFromRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
				})

				It("responds with an Unknown Error", func() {
					expectUnknownError()
				})
			})
		})
	})

	Describe("the DELETE /v3/routes/:guid endpoint", func() {
		BeforeEach(func() {
			requestMethod = http.MethodDelete
			requestPath = "/v3/routes/" + testRouteGUID

			routeRepo.GetRouteReturns(repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				Domain: repositories.DomainRecord{
					Name: testDomainName,
					GUID: testDomainGUID,
				},
			}, nil)
			routeRepo.DeleteRouteReturns(nil)
		})

		When("on the happy path", func() {
			It("responds with a 202 accepted response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			})

			It("responds with a job URL in a location header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/route.delete~"+testRouteGUID))
			})

			It("fetches the right route", func() {
				Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
				_, info, actualRouteGUID := routeRepo.GetRouteArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(actualRouteGUID).To(Equal(testRouteGUID))
			})

			It("deletes the K8s record via the repository", func() {
				Expect(routeRepo.DeleteRouteCallCount()).To(Equal(1))
				_, info, deleteMessage := routeRepo.DeleteRouteArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(deleteMessage.GUID).To(Equal(testRouteGUID))
				Expect(deleteMessage.SpaceGUID).To(Equal(testSpaceGUID))
			})
		})

		When("fetching the route errors", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("deleting the route errors", func() {
			BeforeEach(func() {
				routeRepo.DeleteRouteReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})

func initializeCreateRouteRequestBody(host, path string, spaceGUID, domainGUID string, labels, annotations map[string]string) string {
	marshaledLabels, _ := json.Marshal(labels)
	marshaledAnnotations, _ := json.Marshal(annotations)

	return `{
		"host": "` + host + `",
		"path": "` + path + `",
		"relationships": {
			"domain": {
				"data": {
					"guid": "` + domainGUID + `"
				}
			},
			"space": {
				"data": {
					"guid": "` + spaceGUID + `"
				}
			}
		},
		"metadata": {
			"labels": ` + string(marshaledLabels) + `,
			"annotations": ` + string(marshaledAnnotations) + `
		}
	}`
}
