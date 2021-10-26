package apis_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/gomega/gstruct"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("RouteHandler", func() {
	Describe("the GET /v3/routes/:guid endpoint", func() {
		const (
			testDomainGUID = "test-domain-guid"
			testRouteGUID  = "test-route-guid"
			testRouteHost  = "test-route-host"
			testSpaceGUID  = "test-space-guid"
		)

		var (
			routeRepo     *fake.CFRouteRepository
			domainRepo    *fake.CFDomainRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
		)

		BeforeEach(func() {
			routeRepo = new(fake.CFRouteRepository)
			domainRepo = new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			routeRepo.FetchRouteReturns(repositories.RouteRecord{
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

			domainRepo.FetchDomainReturns(repositories.DomainRecord{
				GUID: testDomainGUID,
				Name: "example.org",
			}, nil)

			routeHandler := NewRouteHandler(
				logf.Log.WithName("TestRouteHandler"),
				*serverURL,
				routeRepo,
				domainRepo,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{}, // required for k8s client (transitive dependency from route repo)
			)
			routeHandler.RegisterRoutes(router)

			var err error
			req, err = http.NewRequest("GET", fmt.Sprintf("/v3/routes/%s", testRouteGUID), nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
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
				Expect(routeRepo.FetchRouteCallCount()).To(Equal(1), "Repo FetchRoute was not called")
				_, _, actualRouteGUID := routeRepo.FetchRouteArgsForCall(0)
				Expect(actualRouteGUID).To(Equal(testRouteGUID), "FetchRoute was not passed the correct GUID")
			})

			It("fetches the correct domain", func() {
				Expect(domainRepo.FetchDomainCallCount()).To(Equal(1), "Repo FetchDomain was not called")
				_, _, actualDomainGUID := domainRepo.FetchDomainArgsForCall(0)
				Expect(actualDomainGUID).To(Equal(testDomainGUID), "FetchDomain was not passed the correct GUID")
			})
		})

		When("the route cannot be found", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteReturns(repositories.RouteRecord{}, repositories.NotFoundError{Err: errors.New("not found")})

				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectNotFoundError("Route not found")
			})
		})

		When("the route's domain cannot be found", func() {
			BeforeEach(func() {
				domainRepo.FetchDomainReturns(repositories.DomainRecord{}, repositories.NotFoundError{Err: errors.New("not found")})

				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is some other error fetching the route", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))

				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/routes endpoint", func() {
		const (
			testDomainGUID = "test-domain-guid"
			testRouteGUID  = "test-route-guid"
			testRouteHost  = "test-route-host"
			testSpaceGUID  = "test-space-guid"
		)

		var (
			routeRepo     *fake.CFRouteRepository
			domainRepo    *fake.CFDomainRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder

			domainRecord *repositories.DomainRecord
			routeRecord  *repositories.RouteRecord
		)

		BeforeEach(func() {

			routeRepo = new(fake.CFRouteRepository)
			domainRepo = new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			routeRecord = &repositories.RouteRecord{
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
			routeRepo.FetchRouteListReturns([]repositories.RouteRecord{
				*routeRecord,
			}, nil)

			domainRecord = &repositories.DomainRecord{
				GUID: testDomainGUID,
				Name: "example.org",
			}
			domainRepo.FetchDomainReturns(*domainRecord, nil)

			routeHandler := NewRouteHandler(
				logf.Log.WithName("TestRouteHandler"),
				*serverURL,
				routeRepo,
				domainRepo,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{}, // required for k8s client (transitive dependency from route repo)
			)
			routeHandler.RegisterRoutes(router)

			var err error
			req, err = http.NewRequest("GET", "/v3/routes", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("on the happy path", func() {
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
						"href": "%[1]s/v3/routes?page=1"
					},
					"last": {
						"href": "%[1]s/v3/routes?page=1"
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

		When("no routes exist", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteListReturns([]repositories.RouteRecord{}, nil)
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
							"href": "%[1]s/v3/routes?page=1"
						},
						"last": {
							"href": "%[1]s/v3/routes?page=1"
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
				routeRepo.FetchRouteListReturns([]repositories.RouteRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is a failure finding a Domain", func() {
			BeforeEach(func() {
				domainRepo.FetchDomainReturns(repositories.DomainRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/routes endpoint", func() {
		const (
			testDomainGUID = "test-domain-guid"
			testDomainName = "test-domain-name"
			testRouteGUID  = "test-route-guid"
			testRouteHost  = "test-route-host"
			testRoutePath  = "/test-route-path"
			testSpaceGUID  = "test-space-guid"
		)

		var (
			routeRepo     *fake.CFRouteRepository
			domainRepo    *fake.CFDomainRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
		)

		makePostRequest := func(requestBody string) {
			req, err := http.NewRequest("POST", "/v3/routes", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		BeforeEach(func() {
			routeRepo = new(fake.CFRouteRepository)
			domainRepo = new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			apiHandler := NewRouteHandler(
				logf.Log.WithName("TestRouteHandler"),
				*serverURL,
				routeRepo,
				domainRepo,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{}, // required for k8s client (transitive dependency from route repo)
			)
			apiHandler.RegisterRoutes(router)
		})

		When("the space exists and the route does not exist and", func() {
			When("a plain POST test route request is sent without metadata", func() {
				BeforeEach(func() {
					appRepo.FetchNamespaceReturns(repositories.SpaceRecord{
						Name: testSpaceGUID,
					}, nil)

					domainRepo.FetchDomainReturns(repositories.DomainRecord{
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

					requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, nil)
					makePostRequest(requestBody)
				})

				It("checks that the specified namespace exists", func() {
					Expect(appRepo.FetchNamespaceCallCount()).To(Equal(1), "Repo FetchNamespace was not called")
					_, _, actualSpaceGUID := appRepo.FetchNamespaceArgsForCall(0)
					Expect(actualSpaceGUID).To(Equal(testSpaceGUID), "FetchNamespace was not passed the correct GUID")
				})

				It("checks that the specified domain exists", func() {
					Expect(domainRepo.FetchDomainCallCount()).To(Equal(1), "Repo FetchDomain was not called")
					_, _, actualDomainGUID := domainRepo.FetchDomainArgsForCall(0)
					Expect(actualDomainGUID).To(Equal(testDomainGUID), "FetchDomain was not passed the correct GUID")
				})

				It("invokes repo CreateRoute with a random GUID", func() {
					Expect(routeRepo.CreateRouteCallCount()).To(Equal(1), "Repo CreateRoute count was not called")
					_, _, createRouteRecord := routeRepo.CreateRouteArgsForCall(0)
					Expect(createRouteRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "CreateRoute record GUID was not a 36 character guid")
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
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

					requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, testLabels, nil)
					makePostRequest(requestBody)
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
					requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, testAnnotations)
					makePostRequest(requestBody)
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
				makePostRequest(`{`)
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
				makePostRequest(`{"description" : "Invalid Request"}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "description"`)
			})
		})

		When("the host is missing", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"relationships": {
						"domain": {
							"data": {
								"guid": "0b78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						},
						"space": {
							"data": {
								"guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Key: 'RouteCreate.Host' Error:Field validation for 'Host' failed on the 'hostname_rfc1123' tag")
			})
		})

		When("the host is not a string", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"host": 12345,
					"relationships": {
						"space": {
							"data": {
								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Host must be a string")
			})
		})

		When("the host format is invalid", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"host": "!-invalid-hostname-!",
					"relationships": {
						"domain": {
							"data": {
								"guid": "0b78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						},
						"space": {
							"data": {
								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Key: 'RouteCreate.Host' Error:Field validation for 'Host' failed on the 'hostname_rfc1123' tag")
			})
		})

		When("the host too long", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"host": "a-really-long-hostname-that-is-not-valid-according-to-the-dns-rfc",
					"relationships": {
						"domain": {
							"data": {
								"guid": "0b78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						},
						"space": {
							"data": {
								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Key: 'RouteCreate.Host' Error:Field validation for 'Host' failed on the 'hostname_rfc1123' tag")
			})
		})

		When("the path is missing a leading /", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"host": "test-route-host",
					"path": "invalid/path",
					 "relationships": {
						"domain": {
							"data": {
								"guid": "0b78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						},
						"space": {
							"data": {
								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Key: 'RouteCreate.Path' Error:Field validation for 'Path' failed on the 'routepathstartswithslash' tag")
			})
		})

		When("the request body is missing the domain relationship", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"host": "test-route-host",
					"relationships": {
						"space": {
							"data": {
								"guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Data is a required field")
			})
		})

		When("the request body is missing the space relationship", func() {
			BeforeEach(func() {
				makePostRequest(`{
					"host": "test-route-host",
					"relationships": {
						"domain": {
							"data": {
								"guid": "0b78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
						}
					}
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Data is a required field")
			})
		})

		When("the client cannot be built", func() {
			BeforeEach(func() {
				clientBuilder.Returns(nil, errors.New("failed to build client"))

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("FetchNamespace returns an unknown error", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
					errors.New("random error"))

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{
					Name: testSpaceGUID,
				}, nil)

				domainRepo.FetchDomainReturns(repositories.DomainRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
				makePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid domain. Ensure that the domain exists and you have access to it.")
			})
		})

		When("FetchDomain returns an unknown error", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{
					Name: testSpaceGUID,
				}, nil)

				domainRepo.FetchDomainReturns(repositories.DomainRecord{},
					errors.New("random error"))

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
				makePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("CreateRoute returns an unknown error", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{
					Name: testSpaceGUID,
				}, nil)

				domainRepo.FetchDomainReturns(repositories.DomainRecord{
					GUID: testDomainGUID,
					Name: testDomainName,
				}, nil)

				routeRepo.CreateRouteReturns(repositories.RouteRecord{},
					errors.New("random error"))

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
				makePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/routes/:guid/destinations endpoint", func() {
		const (
			testDomainGUID = "test-domain-guid"
			testRouteGUID  = "test-route-guid"
			testRouteHost  = "test-route-host"
			testSpaceGUID  = "test-space-guid"
		)

		var (
			routeRepo     *fake.CFRouteRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
			routeRecord   *repositories.RouteRecord
		)

		BeforeEach(func() {
			routeRepo = new(fake.CFRouteRepository)
			domainRepo := new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			routeRecord = &repositories.RouteRecord{
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
					},
					{
						GUID:        "fbef10a2-8ee7-11e9-aa2d-abeeaf7b83c5",
						AppGUID:     "01856e12-8ee8-11e9-98a5-bb397dbc818f",
						ProcessType: "api",
						Port:        9000,
					},
				},
				CreatedAt: "create-time",
				UpdatedAt: "update-time",
			}
			routeRepo.FetchRouteReturns(*routeRecord, nil)

			routeHandler := NewRouteHandler(
				logf.Log.WithName("TestRouteHandler"),
				*serverURL,
				routeRepo,
				domainRepo,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{}, // required for k8s client (transitive dependency from route repo)
			)
			routeHandler.RegisterRoutes(router)

			var err error
			req, err = http.NewRequest("GET", fmt.Sprintf("/v3/routes/%s/destinations", testRouteGUID), nil)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("On the happy path and", func() {
			When("the Route has destinations", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
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
					routeRepo.FetchRouteReturns(
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

		When("the route cannot be found", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteReturns(repositories.RouteRecord{}, repositories.NotFoundError{Err: errors.New("not found")})
			})

			It("returns an error", func() {
				expectNotFoundError("Route not found")
			})
		})

		When("there is some other issue fetching the route", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))
			})

			It("returns an errror", func() {
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
			routeRepo     *fake.CFRouteRepository
			domainRepo    *fake.CFDomainRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
			domain        repositories.DomainRecord
			route         repositories.RouteRecord
		)

		makePostRequest := func(requestBody string, sprintfArgs ...interface{}) {
			req, err := http.NewRequest("POST", "/v3/routes/"+routeGUID+"/destinations",
				strings.NewReader(
					fmt.Sprintf(requestBody, sprintfArgs...),
				),
			)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		BeforeEach(func() {
			routeRepo = new(fake.CFRouteRepository)
			domainRepo = new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			apiHandler := NewRouteHandler(
				logf.Log.WithName("TestRouteHandler"),
				*serverURL,
				routeRepo,
				domainRepo,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{}, // required for k8s client (transitive dependency from route repo)
			)
			apiHandler.RegisterRoutes(router)

			route = repositories.RouteRecord{
				GUID:         routeGUID,
				SpaceGUID:    spaceGUID,
				Domain:       repositories.DomainRecord{GUID: domainGUID},
				Host:         routeHost,
				Path:         "",
				Protocol:     "http1",
				Destinations: nil,
			}

			domain = repositories.DomainRecord{
				GUID: domainGUID,
				Name: "my-tld.com",
			}

			routeRepo.FetchRouteReturns(route, nil)
			domainRepo.FetchDomainReturns(domain, nil)
		})

		When("the request body is valid", func() {
			BeforeEach(func() {
				updatedRoute := route
				updatedRoute.Domain = domain
				updatedRoute.Destinations = []repositories.DestinationRecord{
					{
						GUID:        destination1GUID,
						AppGUID:     destination1AppGUID,
						ProcessType: "web",
						Port:        8080,
					},
					{
						GUID:        destination2GUID,
						AppGUID:     destination2AppGUID,
						ProcessType: destination2ProcessType,
						Port:        destination2Port,
					},
				}
				routeRepo.AddDestinationsToRouteReturns(updatedRoute, nil)
			})

			JustBeforeEach(func() {
				makePostRequest(`{ 
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
				Expect(message.Route).To(Equal(repositories.RouteRecord{
					GUID:         routeGUID,
					SpaceGUID:    spaceGUID,
					Domain:       domain,
					Host:         routeHost,
					Path:         "",
					Protocol:     "http1",
					Destinations: nil,
				}))

				Expect(message.Destinations).To(ConsistOf(
					MatchAllFields(Fields{
						"GUID":        Not(BeEmpty()),
						"AppGUID":     Equal(destination1AppGUID),
						"ProcessType": Equal("web"),
						"Port":        Equal(8080),
					}),
					MatchAllFields(Fields{
						"GUID":        Not(BeEmpty()),
						"AppGUID":     Equal(destination2AppGUID),
						"ProcessType": Equal(destination2ProcessType),
						"Port":        Equal(destination2Port),
					}),
				))
			})

			When("the route doesn't exist", func() {
				BeforeEach(func() {
					routeRepo.FetchRouteReturns(repositories.RouteRecord{}, repositories.NotFoundError{})
				})

				It("responds with 422 and an error", func() {
					expectUnprocessableEntityError("Route is invalid. Ensure it exists and you have access to it.")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("fetching the route errors", func() {
				BeforeEach(func() {
					routeRepo.FetchRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
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
		})

		When("the request body is invalid", func() {
			When("JSON is invalid", func() {
				BeforeEach(func() {
					makePostRequest(`{ this_is_a_invalid_json }`)
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
					makePostRequest(`{
						"destinations": [
						  {
							"port": 9000,
							"protocol": "http1"
						  }
						]
					}`)
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
					makePostRequest(`{
						"destinations": [
						  {
							"app": {},
							"port": 9000,
							"protocol": "http1"
						  }
						]
					}`)
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
					makePostRequest(`{
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
					}`)
				})

				It("returns a status 422 Unprocessable Entity ", func() {
					expectUnprocessableEntityError("Type is a required field")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("destination protocol is not http or http1", func() {
				BeforeEach(func() {
					makePostRequest(`{
						"destinations": [
						  {
							"app": {
							  "guid": "01856e12-8ee8-11e9-98a5-bb397dbc818f"
							},
							"port": 9000,
							"protocol": "xyz"
						  }
						]
					}`)
				})

				It("returns a status 422 Unprocessable Entity ", func() {
					expectUnprocessableEntityError("Protocol must be one of [http http1]")
				})

				It("doesn't add any destinations to a route", func() {
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
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
