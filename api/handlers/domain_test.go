package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domain", func() {
	var (
		apiHandler           *handlers.Domain
		domainRepo           *fake.CFDomainRepository
		requestJSONValidator *fake.RequestJSONValidator
		req                  *http.Request
	)

	BeforeEach(func() {
		requestJSONValidator = new(fake.RequestJSONValidator)
		domainRepo = new(fake.CFDomainRepository)
		apiHandler = handlers.NewDomain(
			*serverURL,
			requestJSONValidator,
			domainRepo,
		)
		apiHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("POST /v3/domain", func() {
		var payload payloads.DomainCreate

		BeforeEach(func() {
			payload = payloads.DomainCreate{
				Name:     "my.domain",
				Internal: false,
				Metadata: payloads.Metadata{
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				},
			}
			requestJSONValidator.DecodeAndValidateJSONPayloadStub = func(_ *http.Request, i interface{}) error {
				domain, ok := i.(*payloads.DomainCreate)
				Expect(ok).To(BeTrue())
				*domain = payload

				return nil
			}

			domainRepo.CreateDomainReturns(repositories.DomainRecord{
				Name:        "my.domain",
				GUID:        "domain-guid",
				Labels:      map[string]string{"foo": "bar"},
				Annotations: map[string]string{"bar": "baz"},
				Namespace:   "my-ns",
				CreatedAt:   "created-on",
				UpdatedAt:   "updated-on",
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/domains", strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		It("has the correct response type", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
		})

		It("invokes create domain correctly", func() {
			Expect(domainRepo.CreateDomainCallCount()).To(Equal(1))
			_, actualAuthInfo, createMessage := domainRepo.CreateDomainArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createMessage.Name).To(Equal(payload.Name))
			Expect(createMessage.Metadata.Labels).To(Equal(map[string]string{
				"foo": "bar",
			}))
			Expect(createMessage.Metadata.Annotations).To(Equal(map[string]string{
				"bar": "baz",
			}))
		})

		It("returns the correct JSON", func() {
			Expect(rr.Body.String()).To(MatchJSON(`
			{
				"name": "my.domain",
				"guid": "domain-guid",
				"internal": false,
				"router_group": null,
				"supported_protocols": [
					"http"
				],
				"created_at": "created-on",
				"updated_at": "updated-on",
				"metadata": {
					"labels": {
						"foo": "bar"
					},
					"annotations": {
						"bar": "baz"
					}
				},
				"relationships": {
					"organization": {
						"data": null
					},
					"shared_organizations": {
						"data": []
					}
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/domains/domain-guid"
					},
					"route_reservations": {
						"href": "https://api.example.org/v3/domains/domain-guid/route_reservations"
					},
					"router_group": null
				}
			}
			`))
		})

		When("decoding the payload fails", func() {
			BeforeEach(func() {
				requestJSONValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "oops"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("oops")
			})
		})

		When("the decoded payload is not valid", func() {
			BeforeEach(func() {
				payload.Internal = true
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Error converting domain payload to repository message: internal domains are not supported")
			})
		})

		When("creating the domain fails", func() {
			BeforeEach(func() {
				domainRepo.CreateDomainReturns(repositories.DomainRecord{}, errors.New("domain-create-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/domains/:guid", func() {
		BeforeEach(func() {
			domainRepo.GetDomainReturns(repositories.DomainRecord{
				Name:        "my.domain",
				GUID:        "domain-guid",
				Labels:      map[string]string{"foo": "bar"},
				Annotations: map[string]string{"bar": "baz"},
				Namespace:   "my-ns",
				CreatedAt:   "created-on",
				UpdatedAt:   "updated-on",
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/domains/domain-guid", strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		It("has the correct response type", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
		})

		It("returns the correct JSON", func() {
			Expect(rr.Body.String()).To(MatchJSON(`
			{
				"name": "my.domain",
				"guid": "domain-guid",
				"internal": false,
				"router_group": null,
				"supported_protocols": [
					"http"
				],
				"created_at": "created-on",
				"updated_at": "updated-on",
				"metadata": {
					"labels": {
						"foo": "bar"
					},
					"annotations": {
						"bar": "baz"
					}
				},
				"relationships": {
					"organization": {
						"data": null
					},
					"shared_organizations": {
						"data": []
					}
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/domains/domain-guid"
					},
					"route_reservations": {
						"href": "https://api.example.org/v3/domains/domain-guid/route_reservations"
					},
					"router_group": null
				}
			}
			`))
		})

		When("the domain repo returns an error", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, errors.New("get-domain-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user is not authorized", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, apierrors.NewForbiddenError(nil, "CFDomain"))
			})

			It("returns 404 NotFound", func() {
				expectNotFoundError("CFDomain not found")
			})
		})
	})

	Describe("PATCH /v3/domains/:guid", func() {
		var payload payloads.DomainUpdate

		BeforeEach(func() {
			payload = payloads.DomainUpdate{
				Metadata: payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"bar": tools.PtrTo("baz"),
					},
				},
			}
			requestJSONValidator.DecodeAndValidateJSONPayloadStub = func(_ *http.Request, i interface{}) error {
				update, ok := i.(*payloads.DomainUpdate)
				Expect(ok).To(BeTrue())
				*update = payload

				return nil
			}

			domainRepo.UpdateDomainReturns(repositories.DomainRecord{
				Name:        "my.domain",
				GUID:        "domain-guid",
				Labels:      map[string]string{"foo": "bar"},
				Annotations: map[string]string{"bar": "baz"},
				Namespace:   "my-ns",
				CreatedAt:   "created-on",
				UpdatedAt:   "updated-on",
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/domains/my-domain", strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		It("has the correct response type", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
		})

		It("returns the correct JSON", func() {
			Expect(rr.Body.String()).To(MatchJSON(`
			{
				"name": "my.domain",
				"guid": "domain-guid",
				"internal": false,
				"router_group": null,
				"supported_protocols": [
					"http"
				],
				"created_at": "created-on",
				"updated_at": "updated-on",
				"metadata": {
					"labels": {
						"foo": "bar"
					},
					"annotations": {
						"bar": "baz"
					}
				},
				"relationships": {
					"organization": {
						"data": null
					},
					"shared_organizations": {
						"data": []
					}
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/domains/domain-guid"
					},
					"route_reservations": {
						"href": "https://api.example.org/v3/domains/domain-guid/route_reservations"
					},
					"router_group": null
				}
			}
			`))
		})

		It("invokes the domain repo correctly", func() {
			Expect(domainRepo.UpdateDomainCallCount()).To(Equal(1))
			_, _, updateMessage := domainRepo.UpdateDomainArgsForCall(0)
			Expect(updateMessage).To(Equal(repositories.UpdateDomainMessage{
				GUID: "my-domain",
				MetadataPatch: repositories.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}))
		})

		When("decoding the payload fails", func() {
			BeforeEach(func() {
				requestJSONValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "oops"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("oops")
			})
		})

		When("the domain repo returns an error", func() {
			BeforeEach(func() {
				domainRepo.UpdateDomainReturns(repositories.DomainRecord{}, errors.New("update-domain-error"))
			})
			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user is not authorized to get domains", func() {
			BeforeEach(func() {
				domainRepo.GetDomainReturns(repositories.DomainRecord{}, apierrors.NewForbiddenError(nil, "CFDomain"))
			})

			It("returns 404 NotFound", func() {
				expectNotFoundError("CFDomain not found")
			})
		})
	})

	Describe("GET /v3/domains", func() {
		var domainRecord *repositories.DomainRecord

		BeforeEach(func() {
			domainRecord = &repositories.DomainRecord{
				GUID:        "test-domain-guid",
				Name:        "example.org",
				Labels:      nil,
				Annotations: nil,
				CreatedAt:   "2019-05-10T17:17:48Z",
				UpdatedAt:   "2019-05-10T17:17:48Z",
			}
			domainRepo.ListDomainsReturns([]repositories.DomainRecord{*domainRecord}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/domains", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 200 OK", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
		})

		It("returns the Pagination Data and Domain Resources in the response", func() {
			Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
				"pagination": {
					"total_results": 1,
					"total_pages": 1,
					"first": {
						"href": "%[1]s/v3/domains"
					},
					"last": {
						"href": "%[1]s/v3/domains"
					},
					"next": null,
					"previous": null
				},
				"resources": [
					 {
					  "guid": "%[2]s",
					  "created_at": "%[3]s",
					  "updated_at": "%[4]s",
					  "name": "%[5]s",
					  "internal": false,
					  "router_group": null,
					  "supported_protocols": ["http"],
					  "metadata": {
						"labels": {},
						"annotations": {}
					  },
					  "relationships": {
						"organization": {
						  "data": null
						},
						"shared_organizations": {
						  "data": []
						}
					  },
					  "links": {
						"self": {
						  "href": "%[1]s/v3/domains/%[2]s"
						},
						"route_reservations": {
						  "href": "%[1]s/v3/domains/%[2]s/route_reservations"
						},
						"router_group": null
					  }
					}
				]
				}`, defaultServerURL, domainRecord.GUID, domainRecord.CreatedAt, domainRecord.UpdatedAt, domainRecord.Name, domainRecord.Name)), "Response body matches response:")
		})

		When("no domain exists", func() {
			BeforeEach(func() {
				domainRepo.ListDomainsReturns([]repositories.DomainRecord{}, nil)
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
							"href": "%[1]s/v3/domains"
						},
						"last": {
							"href": "%[1]s/v3/domains"
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

		When("there is an error listing domains", func() {
			BeforeEach(func() {
				domainRepo.ListDomainsReturns([]repositories.DomainRecord{}, errors.New("unexpected error!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("DELETE /v3/domain", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "DELETE", "/v3/domains/my-domain", &strings.Reader{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes the domain with the repository", func() {
			Expect(domainRepo.DeleteDomainCallCount()).To(Equal(1))
			_, _, deletedDomainGUID := domainRepo.DeleteDomainArgsForCall(0)
			Expect(deletedDomainGUID).To(Equal("my-domain"))
		})

		It("responds with a 202 accepted response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
		})

		It("responds with a job URL in a location header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/domain.delete~my-domain"))
		})

		When("deleting the domain fails", func() {
			BeforeEach(func() {
				domainRepo.DeleteDomainReturns(errors.New("delete-domain-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user does not have permissions to delete domains", func() {
			BeforeEach(func() {
				domainRepo.DeleteDomainReturns(apierrors.NewForbiddenError(nil, "CFDomain"))
			})

			It("returns a not found error", func() {
				expectNotFoundError("CFDomain not found")
			})
		})
	})
})
