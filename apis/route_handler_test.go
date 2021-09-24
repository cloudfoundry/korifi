package apis_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestRoute(t *testing.T) {
	spec.Run(t, "the GET /v3/routes/:guid endpoint", testRouteGetHandler, spec.Report(report.Terminal{}))
	spec.Run(t, "the POST /v3/routes endpoint", testRouteCreateHandler, spec.Report(report.Terminal{}))
}

func testRouteGetHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		testDomainGUID = "test-domain-guid"
		testRouteGUID  = "test-route-guid"
		testRouteName  = "test-route-name"
		testSpaceGUID  = "test-space-guid"
	)

	var (
		rr            *httptest.ResponseRecorder
		routeRepo     *fake.CFRouteRepository
		domainRepo    *fake.CFDomainRepository
		clientBuilder *fake.ClientBuilder
		req           *http.Request
		router        *mux.Router
	)

	it.Before(func() {
		rr = httptest.NewRecorder()
		router = mux.NewRouter()

		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		clientBuilder = new(fake.ClientBuilder)

		routeRepo.FetchRouteReturns(repositories.RouteRecord{
			GUID:      testRouteGUID,
			SpaceGUID: testSpaceGUID,
			DomainRef: repositories.DomainRecord{
				GUID: testDomainGUID,
			},
			Host:      testRouteName,
			Protocol:  "http",
			CreatedAt: "create-time",
			UpdatedAt: "update-time",
		}, nil)

		domainRepo.FetchDomainReturns(repositories.DomainRecord{
			GUID: testDomainGUID,
			Name: "example.org",
		}, nil)

		routeHandler := &RouteHandler{
			ServerURL:   defaultServerURL,
			RouteRepo:   routeRepo,
			DomainRepo:  domainRepo,
			BuildClient: clientBuilder.Spy,
			Logger:      logf.Log.WithName("TestRouteHandler"),
			K8sConfig:   &rest.Config{}, // required for k8s client (transitive dependency from route repo)
		}
		routeHandler.RegisterRoutes(router)

		var err error
		req, err = http.NewRequest("GET", fmt.Sprintf("/v3/routes/%s", testRouteGUID), nil)
		g.Expect(err).NotTo(HaveOccurred())
	})

	getRR := func() *httptest.ResponseRecorder { return rr }

	when("on the happy path", func() {
		it.Before(func() {
			router.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns the Route in the response", func() {
			expectedBody := `{
				"guid":     "test-route-guid",
				"port": null,
				"path": "",
				"protocol": "http",
				"host":     "test-route-name",
				"url":      "test-route-name.example.org",
				"created_at": "create-time",
				"updated_at": "update-time",
				"destinations": null,
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
						"href": "https://api.example.org/v3/routes/test-route-guid"
					},
					"space":{
						"href": "https://api.example.org/v3/spaces/test-space-guid"
					},
					"domain":{
						"href": "https://api.example.org/v3/domains/test-domain-guid"
					},
					"destinations":{
						"href": "https://api.example.org/v3/routes/test-route-guid/destinations"
					}
				}
			}`

			g.Expect(rr.Body.String()).To(MatchJSON(expectedBody), "Response body matches response:")
		})

		it("fetches the correct route", func() {
			g.Expect(routeRepo.FetchRouteCallCount()).To(Equal(1), "Repo FetchRoute was not called")
			_, _, actualRouteGUID := routeRepo.FetchRouteArgsForCall(0)
			g.Expect(actualRouteGUID).To(Equal(testRouteGUID), "FetchRoute was not passed the correct GUID")
		})

		it("fetches the correct domain", func() {
			g.Expect(domainRepo.FetchDomainCallCount()).To(Equal(1), "Repo FetchDomain was not called")
			_, _, actualDomainGUID := domainRepo.FetchDomainArgsForCall(0)
			g.Expect(actualDomainGUID).To(Equal(testDomainGUID), "FetchDomain was not passed the correct GUID")
		})
	})

	when("the route cannot be found", func() {
		it.Before(func() {
			routeRepo.FetchRouteReturns(repositories.RouteRecord{}, repositories.NotFoundError{Err: errors.New("not found")})

			router.ServeHTTP(rr, req)
		})

		itRespondsWithNotFound(it, g, "Route not found", getRR)
	})

	when("the route's domain cannot be found", func() {
		it.Before(func() {
			domainRepo.FetchDomainReturns(repositories.DomainRecord{}, repositories.NotFoundError{Err: errors.New("not found")})

			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("there is some other error fetching the route", func() {
		it.Before(func() {
			routeRepo.FetchRouteReturns(repositories.RouteRecord{}, errors.New("unknown!"))

			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})
}

func testRouteCreateHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		testDomainGUID = "test-domain-guid"
		testDomainName = "test-domain-name"
		testRouteGUID  = "test-route-guid"
		testRouteHost  = "test-route-name"
		testRoutePath  = "/test-route-path"
		testRouteName  = "test-route-name"
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
		g.Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	}

	it.Before(func() {
		rr = httptest.NewRecorder()
		router = mux.NewRouter()

		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		appRepo = new(fake.CFAppRepository)
		clientBuilder = new(fake.ClientBuilder)

		apiHandler := &RouteHandler{
			RouteRepo:   routeRepo,
			DomainRepo:  domainRepo,
			AppRepo:     appRepo,
			BuildClient: clientBuilder.Spy,
			K8sConfig:   &rest.Config{},
			Logger:      logf.Log.WithName("TestRouteHandler"),
			ServerURL:   defaultServerURL,
		}
		apiHandler.RegisterRoutes(router)
	})

	when("the space exists and the route does not exist and", func() {
		when("a plain POST test route request is sent without metadata", func() {
			it.Before(func() {
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
					Host:      testRouteName,
					Path:      testRoutePath,
					Protocol:  "http",
					CreatedAt: "create-time",
					UpdatedAt: "update-time",
				}, nil)

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, nil)
				makePostRequest(requestBody)
			})

			it("checks that the specified namespace exists", func() {
				g.Expect(appRepo.FetchNamespaceCallCount()).To(Equal(1), "Repo FetchNamespace was not called")
				_, _, actualSpaceGUID := appRepo.FetchNamespaceArgsForCall(0)
				g.Expect(actualSpaceGUID).To(Equal(testSpaceGUID), "FetchNamespace was not passed the correct GUID")
			})

			it("checks that the specified domain exists", func() {
				g.Expect(domainRepo.FetchDomainCallCount()).To(Equal(1), "Repo FetchDomain was not called")
				_, _, actualDomainGUID := domainRepo.FetchDomainArgsForCall(0)
				g.Expect(actualDomainGUID).To(Equal(testDomainGUID), "FetchDomain was not passed the correct GUID")
			})

			it("invokes repo CreateRoute with a random GUID", func() {
				g.Expect(routeRepo.CreateRouteCallCount()).To(Equal(1), "Repo CreateRoute count was not called")
				_, _, createRouteRecord := routeRepo.CreateRouteArgsForCall(0)
				g.Expect(createRouteRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "CreateRoute record GUID was not a 36 character guid")
			})

			it("returns status 200 OK", func() {
				g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			it("returns Content-Type as JSON in header", func() {
				g.Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			it("returns the \"created route\"(the mock response record) in the response", func() {
				g.Expect(rr.Body.String()).To(MatchJSON(`{
					  "guid": "test-route-guid",
					  "protocol": "http",
					  "port": null,
					  "host": "test-route-name",
					  "path": "/test-route-path",
					  "url": "test-route-name.test-domain-name/test-route-path",
					  "created_at": "create-time",
					  "updated_at": "update-time",
					  "destinations": null,
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
							   "href": "https://api.example.org/v3/routes/test-route-guid"
						  },
						  "space": {
							   "href": "https://api.example.org/v3/spaces/test-space-guid"
						  },
						  "domain": {
							   "href": "https://api.example.org/v3/domains/test-domain-guid"
						  },
						  "destinations": {
							   "href": "https://api.example.org/v3/routes/test-route-guid/destinations"
						  }
					  }
				 }`), "Response body mismatch")
			})
		})

		when("a POST test route request is sent with metadata labels", func() {
			var (
				testLabels map[string]string
			)

			it.Before(func() {
				testLabels = map[string]string{"label1": "foo", "label2": "bar"}

				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, testLabels, nil)
				makePostRequest(requestBody)
			})

			it("should pass along the labels to CreateRoute", func() {
				g.Expect(routeRepo.CreateRouteCallCount()).To(Equal(1), "Repo CreateRoute count was not invoked 1 time")
				_, _, createRouteRecord := routeRepo.CreateRouteArgsForCall(0)
				g.Expect(createRouteRecord.Labels).To(Equal(testLabels))
			})
		})

		when("a POST test route request is sent with metadata annotations", func() {
			var (
				testAnnotations map[string]string
			)

			it.Before(func() {
				testAnnotations = map[string]string{"annotation1": "foo", "annotation2": "bar"}
				requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, testAnnotations)
				makePostRequest(requestBody)
			})

			it("should pass along the annotations to CreateRoute", func() {
				g.Expect(routeRepo.CreateRouteCallCount()).To(Equal(1), "Repo CreateRoute count was not invoked 1 time")
				_, _, createRouteRecord := routeRepo.CreateRouteArgsForCall(0)
				g.Expect(createRouteRecord.Annotations).To(Equal(testAnnotations))
			})
		})
	})

	when("the request body is invalid JSON", func() {
		it.Before(func() {
			makePostRequest(`{`)
		})

		it("returns a status 400 Bad Request ", func() {
			g.Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			g.Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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

	when("the request body includes an unknown description field", func() {
		it.Before(func() {
			makePostRequest(`{"description" : "Invalid Request"}`)
		})

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			g.Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				 "errors": [
					  {
						  "detail": "invalid request body: json: unknown field \"description\"",
						  "title": "CF-UnprocessableEntity",
						  "code": 10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("the request body includes an invalid route host", func() {
		it.Before(func() {
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

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			g.Expect(rr.Header().Get("Content-Type")).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				 "errors": [
					  {
						  "detail": "Host must be a string",
						  "title": "CF-UnprocessableEntity",
						  "code":   10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("the request body is missing the host", func() {
		it.Before(func() {
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

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				 "errors": [
					  {
						  "title": "CF-UnprocessableEntity",
						  "detail": "Host is a required field",
						  "code": 10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("the request body is missing the domain relationship", func() {
		it.Before(func() {
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

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				 "errors": [
					  {
						  "title": "CF-UnprocessableEntity",
						  "detail": "Data is a required field",
						  "code": 10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("the request body is missing the space relationship", func() {
		it.Before(func() {
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

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				 "errors": [
					  {
						  "title": "CF-UnprocessableEntity",
						  "detail": "Data is a required field",
						  "code": 10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("the client cannot be built", func() {
		it.Before(func() {
			clientBuilder.Returns(nil, errors.New("failed to build client"))

			requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, testDomainGUID, nil, nil)
			makePostRequest(requestBody)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("the space does not exist", func() {
		it.Before(func() {
			appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
				repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

			requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
			makePostRequest(requestBody)
		})

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
				 "errors": [
					  {
						  "title": "CF-UnprocessableEntity",
						  "detail": "Invalid space. Ensure that the space exists and you have access to it.",
						  "code": 10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("FetchNamespace returns an unknown error", func() {
		it.Before(func() {
			appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
				errors.New("random error"))

			requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, "no-such-space", testDomainGUID, nil, nil)
			makePostRequest(requestBody)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("the domain does not exist", func() {
		it.Before(func() {
			appRepo.FetchNamespaceReturns(repositories.SpaceRecord{
				Name: testSpaceGUID,
			}, nil)

			domainRepo.FetchDomainReturns(repositories.DomainRecord{},
				repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

			requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
			makePostRequest(requestBody)
		})

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).Should(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).Should(MatchJSON(`{
				 "errors": [
					  {
						  "title": "CF-UnprocessableEntity",
						  "detail": "Invalid domain. Ensure that the domain exists and you have access to it.",
						  "code": 10008
					  }
				 ]
			 }`), "Response body matches response:")
		})
	})

	when("FetchDomain returns an unknown error", func() {
		it.Before(func() {
			appRepo.FetchNamespaceReturns(repositories.SpaceRecord{
				Name: testSpaceGUID,
			}, nil)

			domainRepo.FetchDomainReturns(repositories.DomainRecord{},
				errors.New("random error"))

			requestBody := initializeCreateRouteRequestBody(testRouteHost, testRoutePath, testSpaceGUID, "no-such-domain", nil, nil)
			makePostRequest(requestBody)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("CreateRoute returns an unknown error", func() {
		it.Before(func() {
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

		itRespondsWithUnknownError(it, g, getRR)
	})
}

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
