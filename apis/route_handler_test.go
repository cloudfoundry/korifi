package apis_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/gorilla/mux"
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
			rr            *httptest.ResponseRecorder
			routeRepo     *fake.CFRouteRepository
			domainRepo    *fake.CFDomainRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
			req           *http.Request
			router        *mux.Router
		)

		BeforeEach(func() {
			rr = httptest.NewRecorder()
			router = mux.NewRouter()

			routeRepo = new(fake.CFRouteRepository)
			domainRepo = new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			routeRepo.FetchRouteReturns(repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				DomainRef: repositories.DomainRecord{
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

			serverURL, err := url.Parse(defaultServerURL)
			Expect(err).NotTo(HaveOccurred())
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

			req, err = http.NewRequest("GET", fmt.Sprintf("/v3/routes/%s", testRouteGUID), nil)
			Expect(err).NotTo(HaveOccurred())
		})

		getRR := func() *httptest.ResponseRecorder { return rr }

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

			itRespondsWithNotFound("Route not found", getRR)
		})

		When("the route's domain cannot be found", func() {
			BeforeEach(func() {
				domainRepo.FetchDomainReturns(repositories.DomainRecord{}, repositories.NotFoundError{Err: errors.New("not found")})

				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnknownError(getRR)
		})

		When("there is some other error fetching the route", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))

				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnknownError(getRR)
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
			rr            *httptest.ResponseRecorder
			router        *mux.Router
			routeRepo     *fake.CFRouteRepository
			domainRepo    *fake.CFDomainRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
		)

		getRR := func() *httptest.ResponseRecorder { return rr }

		makePostRequest := func(requestBody string) {
			req, err := http.NewRequest("POST", "/v3/routes", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		BeforeEach(func() {
			rr = httptest.NewRecorder()
			router = mux.NewRouter()

			routeRepo = new(fake.CFRouteRepository)
			domainRepo = new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			serverURL, err := url.Parse(defaultServerURL)
			Expect(err).NotTo(HaveOccurred())
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
						DomainRef: repositories.DomainRecord{
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

			itRespondsWithUnprocessableEntity(`invalid request body: json: unknown field "description"`, getRR)
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

			itRespondsWithUnprocessableEntity(
				"Key: 'RouteCreate.Host' Error:Field validation for 'Host' failed on the 'hostname_rfc1123' tag",
				getRR,
			)
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

			itRespondsWithUnprocessableEntity("Host must be a string", getRR)
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

			itRespondsWithUnprocessableEntity(
				"Key: 'RouteCreate.Host' Error:Field validation for 'Host' failed on the 'hostname_rfc1123' tag",
				getRR,
			)
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

			itRespondsWithUnprocessableEntity(
				"Key: 'RouteCreate.Host' Error:Field validation for 'Host' failed on the 'hostname_rfc1123' tag",
				getRR,
			)
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

			itRespondsWithUnprocessableEntity(
				"Key: 'RouteCreate.Path' Error:Field validation for 'Path' failed on the 'routepathstartswithslash' tag",
				getRR,
			)
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

			itRespondsWithUnprocessableEntity("Data is a required field", getRR)
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

			itRespondsWithUnprocessableEntity("Data is a required field", getRR)
		})

		When("the client cannot be built", func() {
			BeforeEach(func() {
				clientBuilder.Returns(nil, errors.New("failed to build client"))

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			itRespondsWithUnknownError(getRR)
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			itRespondsWithUnprocessableEntity(
				"Invalid space. Ensure that the space exists and you have access to it.",
				getRR,
			)
		})

		When("FetchNamespace returns an unknown error", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
					errors.New("random error"))

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			itRespondsWithUnknownError(getRR)
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

			itRespondsWithUnprocessableEntity(
				"Invalid domain. Ensure that the domain exists and you have access to it.",
				getRR,
			)
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

			itRespondsWithUnknownError(getRR)
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

			itRespondsWithUnknownError(getRR)
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
			rr            *httptest.ResponseRecorder
			routeRepo     *fake.CFRouteRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
			req           *http.Request
			router        *mux.Router
			routeRecord   *repositories.RouteRecord
		)

		getRR := func() *httptest.ResponseRecorder { return rr }

		BeforeEach(func() {
			rr = httptest.NewRecorder()
			router = mux.NewRouter()

			routeRepo = new(fake.CFRouteRepository)
			domainRepo := new(fake.CFDomainRepository)
			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)

			routeRecord = &repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				DomainRef: repositories.DomainRecord{
					GUID: testDomainGUID,
				},
				Host:     testRouteHost,
				Protocol: "http",
				Destinations: []repositories.Destination{
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

			serverURL, err := url.Parse(defaultServerURL)
			Expect(err).NotTo(HaveOccurred())
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
							DomainRef: repositories.DomainRecord{
								GUID: testDomainGUID,
							},
							Host:         testRouteHost,
							Protocol:     "http",
							Destinations: []repositories.Destination{},
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

			itRespondsWithNotFound("Route not found", getRR)
		})

		When("there is some other issue fetching the route", func() {
			BeforeEach(func() {
				routeRepo.FetchRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))
			})

			itRespondsWithUnknownError(getRR)
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
