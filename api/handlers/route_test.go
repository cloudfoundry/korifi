package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Route", func() {
	var (
		routeRepo  *fake.CFRouteRepository
		domainRepo *fake.CFDomainRepository
		appRepo    *fake.CFAppRepository
		spaceRepo  *fake.SpaceRepository

		requestMethod string
		requestPath   string
		requestBody   string
		routeRecord   repositories.RouteRecord
	)

	BeforeEach(func() {
		routeRecord = repositories.RouteRecord{
			GUID:      "test-route-guid",
			SpaceGUID: "test-space-guid",
			Domain: repositories.DomainRecord{
				GUID: "test-domain-guid",
			},
			Host:     "test-route-host",
			Path:     "/some_path",
			Protocol: "http",
			Destinations: []repositories.DestinationRecord{
				{GUID: "dest-1-guid"},
				{GUID: "dest-2-guid"},
			},
			Labels:      nil,
			Annotations: nil,
			CreatedAt:   "2019-05-10T17:17:48Z",
			UpdatedAt:   "2019-05-10T17:17:48Z",
		}
		routeRepo = new(fake.CFRouteRepository)
		routeRepo.GetRouteReturns(routeRecord, nil)

		domainRepo = new(fake.CFDomainRepository)
		domainRepo.GetDomainReturns(repositories.DomainRecord{
			GUID: "test-domain-guid",
			Name: "example.org",
		}, nil)

		appRepo = new(fake.CFAppRepository)

		spaceRepo = new(fake.SpaceRepository)
		spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
			Name: "test-space-guid",
		}, nil)

		decoderValidator, err := NewGoPlaygroundValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewRoute(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/routes/:guid endpoint", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestPath = "/v3/routes/test-route-guid"
			requestBody = ""
		})

		It("returns the route", func() {
			Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, actualRouteGUID := routeRepo.GetRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualRouteGUID).To(Equal("test-route-guid"))

			Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDomainGUID).To(Equal("test-domain-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-route-guid"),
				MatchJSONPath("$.url", "test-route-host.example.org/some_path"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/routes/test-route-guid"),
			)))
		})

		When("the route is not accessible", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Route")
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
		BeforeEach(func() {
			otherRouteRecord := routeRecord
			otherRouteRecord.GUID = "other-test-route-guid"
			otherRouteRecord.Host = "other-test-route-host"
			routeRepo.ListRoutesReturns([]repositories.RouteRecord{
				routeRecord,
				otherRouteRecord,
			}, nil)

			requestMethod = http.MethodGet
			requestPath = "/v3/routes"
			requestBody = ""
		})

		It("returns the routes list", func() {
			Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := routeRepo.ListRoutesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDomainGUID).To(Equal("test-domain-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)),
				MatchJSONPath("$.resources[0].guid", "test-route-guid"),
				MatchJSONPath("$.resources[0].url", "test-route-host.example.org/some_path"),
				MatchJSONPath("$.resources[1].guid", "other-test-route-guid"),
				MatchJSONPath("$.resources[1].url", "other-test-route-host.example.org/some_path"),
			)))
		})

		When("query parameters are provided", func() {
			BeforeEach(func() {
				requestPath += "?app_guids=my-app-guid&" +
					"space_guids=my-space-guid&" +
					"domain_guids=my-domain-guid&" +
					"hosts=my-host&" +
					"paths=/some/path"
			})

			It("filters routes by that", func() {
				Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
				_, _, message := routeRepo.ListRoutesArgsForCall(0)
				Expect(message.AppGUIDs).To(ConsistOf("my-app-guid"))
				Expect(message.SpaceGUIDs).To(ConsistOf("my-space-guid"))
				Expect(message.DomainGUIDs).To(ConsistOf("my-domain-guid"))
				Expect(message.Hosts).To(ConsistOf("my-host"))
				Expect(message.Paths).To(ConsistOf("/some/path"))

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(
					MatchJSONPath(
						"$.pagination.first.href",
						"https://api.example.org/v3/routes?"+
							"app_guids=my-app-guid&"+
							"space_guids=my-space-guid&"+
							"domain_guids=my-domain-guid&"+
							"hosts=my-host&"+
							"paths=/some/path",
					),
				))
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

			routeRepo.CreateRouteReturns(repositories.RouteRecord{
				GUID:      "test-route-guid",
				SpaceGUID: "test-space-guid",
				Domain: repositories.DomainRecord{
					GUID: "test-domain-guid",
				},
				Host:      "test-route-host",
				Path:      "/test-route-path",
				Protocol:  "http",
				CreatedAt: "create-time",
				UpdatedAt: "update-time",
			}, nil)

			requestBody = `{
				"host": "test-route-host",
				"path": "/test-route-path",
				"relationships": {
					"domain": {
						"data": {
							"guid": "test-domain-guid"
						}
					},
					"space": {
						"data": {
							"guid": "test-space-guid"
						}
					}
				},
				"metadata": {
					"labels": {
						"label-key": "label-val"
					},
					"annotations": {
						"annotation-key": "annotation-val"
					}
				}
			}`
		})

		It("creates the route", func() {
			Expect(spaceRepo.GetSpaceCallCount()).To(Equal(1))
			_, actualAuthInfo, actualSpaceGUID := spaceRepo.GetSpaceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualSpaceGUID).To(Equal("test-space-guid"))

			Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDomainGUID).To(Equal("test-domain-guid"))

			Expect(routeRepo.CreateRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, createRouteMessage := routeRepo.CreateRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createRouteMessage.Annotations).To(Equal(map[string]string{"annotation-key": "annotation-val"}))
			Expect(createRouteMessage.DomainGUID).To(Equal("test-domain-guid"))
			Expect(createRouteMessage.Host).To(Equal("test-route-host"))
			Expect(createRouteMessage.Labels).To(Equal(map[string]string{"label-key": "label-val"}))
			Expect(createRouteMessage.SpaceGUID).To(Equal("test-space-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-route-guid"),
				MatchJSONPath("$.url", "test-route-host.example.org/test-route-path"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/routes/test-route-guid"),
			)))
		})

		When("the request body is invalid JSON", func() {
			BeforeEach(func() {
				requestBody = `{`
			})

			It("returns a message parse erorr", func() {
				expectBadRequestError()
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
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("the space is forbidden", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{},
					apierrors.NewForbiddenError(errors.New("not found"), repositories.SpaceResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("GetSpace returns an unknown error", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{},
					errors.New("random error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, apierrors.NewNotFoundError(nil, repositories.DomainResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid domain. Ensure that the domain exists and you have access to it.")
			})
		})

		When("GetDomain returns an unknown error", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, errors.New("random error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("CreateRoute returns an unknown error", func() {
			BeforeEach(func() {
				routeRepo.CreateRouteReturns(repositories.RouteRecord{},
					errors.New("random error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/routes/:guid endpoint", func() {
		BeforeEach(func() {
			requestMethod = "PATCH"
			requestPath = "/v3/routes/test-route-guid"
		})

		BeforeEach(func() {
			routeRepo.PatchRouteMetadataReturns(repositories.RouteRecord{
				GUID:      "test-route-guid",
				SpaceGUID: spaceGUID,
				Labels: map[string]string{
					"env":                           "production",
					"foo.example.com/my-identifier": "aruba",
				},
				Annotations: map[string]string{
					"hello":                       "there",
					"foo.example.com/lorem-ipsum": "Lorem ipsum.",
				},
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

		It("patches the route", func() {
			Expect(routeRepo.PatchRouteMetadataCallCount()).To(Equal(1))
			_, _, msg := routeRepo.PatchRouteMetadataArgsForCall(0)
			Expect(msg.RouteGUID).To(Equal("test-route-guid"))
			Expect(msg.SpaceGUID).To(Equal(spaceGUID))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-route-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/routes/test-route-guid"),
			)))
		})

		When("the user doesn't have permission to get the Route", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns a not found error and doesn't try patching", func() {
				Expect(routeRepo.PatchRouteMetadataCallCount()).To(Equal(0))
				expectNotFoundError("Route")
			})
		})

		When("fetching the Route errors", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
			})

			It("returns an error and doesn't try patching", func() {
				Expect(routeRepo.PatchRouteMetadataCallCount()).To(Equal(0))
				expectUnknownError()
			})
		})

		When("patching the Route errors", func() {
			BeforeEach(func() {
				routeRepo.PatchRouteMetadataReturns(repositories.RouteRecord{}, errors.New("boom"))
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
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestPath = fmt.Sprintf("/v3/routes/%s/destinations", "test-route-guid")
			requestBody = ""
		})

		It("returns the list of destinations", func() {
			Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := routeRepo.GetRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.destinations[0].guid", "dest-1-guid"),
				MatchJSONPath("$.destinations[1].guid", "dest-2-guid"),
			)))
		})

		When("the route is not accessible", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Route")
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
				expectNotFoundError("Route")
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
		BeforeEach(func() {
			updatedRoute := routeRecord
			updatedRoute.Destinations[0].GUID = "new-dest-1-guid"
			updatedRoute.Destinations[1].GUID = "new-dest-2-guid"
			routeRepo.AddDestinationsToRouteReturns(updatedRoute, nil)

			requestMethod = http.MethodPost
			requestPath = "/v3/routes/test-route-guid/destinations"
			requestBody = `{
				"destinations": [
					{
						"app": {
							"guid": "app-1-guid"
						}
					},
					{
						"app": {
							"guid": "app-2-guid",
							"process": {
								"type": "queue"
							}
						},
						"port": 1234,
						"protocol": "http1"
					}
				]
			}`
		})

		It("adds the destinations to the route", func() {
			Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, actualRouteGUID := routeRepo.GetRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualRouteGUID).To(Equal("test-route-guid"))

			Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDomainGUID).To(Equal("test-domain-guid"))

			Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, message := routeRepo.AddDestinationsToRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(message.RouteGUID).To(Equal("test-route-guid"))
			Expect(message.SpaceGUID).To(Equal("test-space-guid"))
			Expect(message.NewDestinations).To(ConsistOf(
				MatchAllFields(Fields{
					"AppGUID":     Equal("app-1-guid"),
					"ProcessType": Equal("web"),
					"Port":        Equal(8080),
					"Protocol":    Equal("http1"),
				}),
				MatchAllFields(Fields{
					"AppGUID":     Equal("app-2-guid"),
					"ProcessType": Equal("queue"),
					"Port":        Equal(1234),
					"Protocol":    Equal("http1"),
				}),
			))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.destinations", HaveLen(2)),
				MatchJSONPath("$.destinations[0].guid", "new-dest-1-guid"),
				MatchJSONPath("$.destinations[1].guid", "new-dest-2-guid"),
			)))
		})

		When("the route doesn't exist", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewNotFoundError(nil, repositories.RouteResourceType))
			})

			It("returns not found and doesn't add the destinations", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				expectNotFoundError("Route")
			})
		})

		When("the user lacks permission to fetch the route", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("returns not found and doesn't add the destinations", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				expectNotFoundError("Route")
			})
		})

		When("fetching the route errors", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
			})

			It("responds with an Unknown Error and doesn't try to add the destinations", func() {
				expectUnknownError()
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

		When("JSON is invalid", func() {
			BeforeEach(func() {
				requestBody = `{`
			})

			It("returns an error", func() {
				expectBadRequestError()
			})
		})

		When("app is missing", func() {
			BeforeEach(func() {
				requestBody = `{
					"destinations": [
						{ "port": 9000, "protocol": "http1" }
					]
				}`
			})

			It("returns an error and doesn't add the destinations", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				expectUnprocessableEntityError("App is a required field")
			})
		})

		When("app GUID is missing", func() {
			BeforeEach(func() {
				requestBody = `{
					"destinations": [
						{ "app": {}, "port": 9000, "protocol": "http1" }
					]
				}`
			})

			It("returns an error and doesn't add the destinations", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				expectUnprocessableEntityError("GUID is a required field")
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

			It("returns an error and doesn't add the destinations", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				expectUnprocessableEntityError("Type is a required field")
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

			It("returns an error and doesn't add the destinations", func() {
				expectUnprocessableEntityError("Protocol must be one of [http1]")
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
			})
		})
	})

	Describe("the DELETE /v3/routes/:guid/destinations/:destination_guid endpoint", func() {
		BeforeEach(func() {
			requestMethod = http.MethodDelete
			requestPath = "/v3/routes/test-route-guid/destinations/test-destination-guid"
			requestBody = ""
		})

		It("deletes the destination", func() {
			Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, actualRouteGUID := routeRepo.GetRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualRouteGUID).To(Equal("test-route-guid"))

			Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(1))
			_, actualAuthInfo, message := routeRepo.RemoveDestinationFromRouteArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(message.RouteGUID).To(Equal("test-route-guid"))
			Expect(message.SpaceGUID).To(Equal("test-space-guid"))
			Expect(message.DestinationGuid).To(Equal("test-destination-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
			Expect(rr).To(HaveHTTPBody(BeEmpty()))
		})

		When("the route doesn't exist", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewNotFoundError(nil, repositories.RouteResourceType))
			})

			It("responds with 404 and an error and doesn't try to delete the destination", func() {
				expectNotFoundError("Route")
				Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(0))
			})
		})

		When("the user lacks permission to fetch the route", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, apierrors.NewForbiddenError(nil, repositories.RouteResourceType))
			})

			It("responds with a 404 error and doesn't try to delete the destination", func() {
				expectNotFoundError("Route")
				Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(0))
			})
		})

		When("fetching the route errors", func() {
			BeforeEach(func() {
				routeRepo.GetRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
			})

			It("responds with an Unknown Error and doesn't try to delete the route", func() {
				expectUnknownError()
				Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(0))
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

	Describe("the DELETE /v3/routes/:guid endpoint", func() {
		BeforeEach(func() {
			requestMethod = http.MethodDelete
			requestPath = "/v3/routes/test-route-guid"
		})

		It("deletes the route", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/route.delete~test-route-guid"))

			Expect(routeRepo.GetRouteCallCount()).To(Equal(1))
			_, info, actualRouteGUID := routeRepo.GetRouteArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(actualRouteGUID).To(Equal("test-route-guid"))

			Expect(routeRepo.DeleteRouteCallCount()).To(Equal(1))
			_, info, deleteMessage := routeRepo.DeleteRouteArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(deleteMessage.GUID).To(Equal("test-route-guid"))
			Expect(deleteMessage.SpaceGUID).To(Equal("test-space-guid"))
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
