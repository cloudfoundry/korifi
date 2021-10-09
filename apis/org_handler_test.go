package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	rootURL  = "https://api.example.org"
	orgsBase = "/v3/organizations"
)

var _ = Describe("OrgHandler", func() {
	var (
		ctx        context.Context
		router     *mux.Router
		orgHandler *apis.OrgHandler
		orgRepo    *fake.CFOrgRepository
		req        *http.Request
		rr         *httptest.ResponseRecorder
		err        error
		now        time.Time
	)

	BeforeEach(func() {
		now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z
	})

	Describe("Create Org", func() {
		makePostRequest := func(requestBody string) {
			req, err := http.NewRequestWithContext(ctx, "POST", orgsBase, strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		BeforeEach(func() {
			ctx = context.Background()
			orgRepo = new(fake.CFOrgRepository)
			orgRepo.CreateOrgStub = func(_ context.Context, record repositories.OrgRecord) (repositories.OrgRecord, error) {
				record.GUID = "t-h-e-o-r-g"
				record.CreatedAt = now
				record.UpdatedAt = now
				return record, nil
			}

			serverURL, err := url.Parse(defaultServerURL)
			Expect(err).NotTo(HaveOccurred())

			orgHandler = apis.NewOrgHandler(orgRepo, *serverURL)
			router = mux.NewRouter()
			orgHandler.RegisterRoutes(router)

			rr = httptest.NewRecorder()
		})

		When("happy path", func() {
			BeforeEach(func() {
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns 201 with appropriate success JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
          "guid": "t-h-e-o-r-g",
					"name": "the-org",
					"created_at": "2021-09-17T15:23:10Z",
					"updated_at": "2021-09-17T15:23:10Z",
					"suspended": false,
					"metadata": {
					  "labels": {},
					  "annotations": {}
					},
					"relationships": {},
					"links": {
						"self": {
							"href": "%[1]s/v3/organizations/t-h-e-o-r-g"
						}
					}
				}`, defaultServerURL))))
			})

			It("invokes the repo org create function with expected parameters", func() {
				Expect(orgRepo.CreateOrgCallCount()).To(Equal(1))
				_, orgRecord := orgRepo.CreateOrgArgsForCall(0)
				Expect(orgRecord.Name).To(Equal("the-org"))
				Expect(orgRecord.Suspended).To(BeFalse())
				Expect(orgRecord.Labels).To(BeEmpty())
				Expect(orgRecord.Annotations).To(BeEmpty())
			})
		})

		When("the org repo returns an error", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, errors.New("boom"))
				makePostRequest(`{"name": "the-org"}`)
			})

			itRespondsWithUnknownError(func() *httptest.ResponseRecorder { return rr })
		})

		When("the user passes optional org parameters", func() {
			BeforeEach(func() {
				makePostRequest(`{
          "name": "the-org",
					"suspended": true,
					"metadata": {
						"labels": {"foo": "bar"},
						"annotations": {"bar": "baz"}
					}
				}`)
			})

			It("invokes the repo org create function with expected parameters", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(orgRepo.CreateOrgCallCount()).To(Equal(1))
				_, orgRecord := orgRepo.CreateOrgArgsForCall(0)
				Expect(orgRecord.Name).To(Equal("the-org"))
				Expect(orgRecord.Suspended).To(BeTrue())
				Expect(orgRecord.Labels).To(And(HaveLen(1), HaveKeyWithValue("foo", "bar")))
				Expect(orgRecord.Annotations).To(And(HaveLen(1), HaveKeyWithValue("bar", "baz")))
			})

			It("returns 201 with appropriate success JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
          "guid": "t-h-e-o-r-g",
					"name": "the-org",
					"created_at": "2021-09-17T15:23:10Z",
					"updated_at": "2021-09-17T15:23:10Z",
					"suspended": true,
					"metadata": {
					  "labels": {"foo": "bar"},
					  "annotations": {"bar": "baz"}
					},
					"relationships": {},
					"links": {
						"self": {
							"href": "%[1]s/v3/organizations/t-h-e-o-r-g"
						}
					}
				}`, defaultServerURL))))
			})
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				makePostRequest(`{`)
			})

			It("returns a status 400 with appropriate error JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
          "errors": [
            {
              "title": "CF-MessageParseError",
              "detail": "Request invalid due to parse error: invalid request body",
              "code": 1001
            }
          ]
        }`)))
			})
		})

		When("the request body has an unknown field", func() {
			BeforeEach(func() {
				makePostRequest(`{"description" : "Invalid Request"}`)
			})

			It("returns a status 422 with appropriate error JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
          "errors": [
            {
              "title": "CF-UnprocessableEntity",
              "detail": "invalid request body: json: unknown field \"description\"",
              "code": 10008
            }
          ]
        }`)))
			})
		})

		When("the request body is invalid with invalid app name", func() {
			BeforeEach(func() {
				makePostRequest(`{"name": 12345}`)
			})

			It("returns a status 422 with appropriate error JSON", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
          "errors": [
            {
              "code":   10008,
              "title": "CF-UnprocessableEntity",
              "detail": "Name must be a string"
            }
          ]
        }`)))
			})
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
				makePostRequest(`{"metadata": {"labels": {"foo": "bar"}}}`)
			})

			It("returns a status 422 with appropriate error message json", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
          "errors": [
            {
              "title": "CF-UnprocessableEntity",
              "detail": "Name is a required field",
              "code": 10008
            }
          ]
        }`)))
			})
		})
	})

	Describe("Listing Orgs", func() {
		BeforeEach(func() {
			ctx = context.Background()
			orgRepo = new(fake.CFOrgRepository)

			now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z
			orgRepo.FetchOrgsReturns([]repositories.OrgRecord{
				{
					Name:      "alice",
					GUID:      "a-l-i-c-e",
					CreatedAt: now,
					UpdatedAt: now,
				},
				{
					Name:      "bob",
					GUID:      "b-o-b",
					CreatedAt: now,
					UpdatedAt: now,
				},
			}, nil)

			serverURL, err := url.Parse(rootURL)
			Expect(err).NotTo(HaveOccurred())
			orgHandler = apis.NewOrgHandler(orgRepo, *serverURL)
			router = mux.NewRouter()
			orgHandler.RegisterRoutes(router)

			rr = httptest.NewRecorder()
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, orgsBase, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("happy path", func() {
			BeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns 200", func() {
				Expect(rr.Result().StatusCode).To(Equal(http.StatusOK))
			})

			It("sets json content type", func() {
				Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
			})

			It("lists orgs using the repository", func() {
				Expect(orgRepo.FetchOrgsCallCount()).To(Equal(1))
				_, names := orgRepo.FetchOrgsArgsForCall(0)
				Expect(names).To(BeEmpty())
			})

			It("renders the orgs response", func() {
				expectedBody := fmt.Sprintf(`
          {
             "pagination": {
                "total_results": 2,
                "total_pages": 1,
                "first": {
                   "href": "%[1]s/v3/organizations?page=1"
                },
                "last": {
                   "href": "%[1]s/v3/organizations?page=1"
                },
                "next": null,
                "previous": null
             },
             "resources": [
                  {
                      "guid": "a-l-i-c-e",
                      "name": "alice",
                      "created_at": "2021-09-17T15:23:10Z",
                      "updated_at": "2021-09-17T15:23:10Z",
                      "suspended": false,
                      "metadata": {
                        "labels": {},
                        "annotations": {}
                      },
                      "relationships": {},
                      "links": {
                          "self": {
                              "href": "%[1]s/v3/organizations/a-l-i-c-e"
                          }
                      }
                  },
                  {
                      "guid": "b-o-b",
                      "name": "bob",
                      "created_at": "2021-09-17T15:23:10Z",
                      "updated_at": "2021-09-17T15:23:10Z",
                      "suspended": false,
                      "metadata": {
                        "labels": {},
                        "annotations": {}
                      },
                      "relationships": {},
                      "links": {
                          "self": {
                              "href": "%[1]s/v3/organizations/b-o-b"
                          }
                      }
                  }
              ]
          }`, rootURL)
				Expect(rr.Body.String()).To(MatchJSON(expectedBody))
			})
		})

		When("names are specified", func() {
			BeforeEach(func() {
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, orgsBase+"?names=foo,bar", nil)
				Expect(err).NotTo(HaveOccurred())

				router.ServeHTTP(rr, req)
			})

			It("filters by them", func() {
				Expect(orgRepo.FetchOrgsCallCount()).To(Equal(1))
				_, names := orgRepo.FetchOrgsArgsForCall(0)
				Expect(names).To(ConsistOf("foo", "bar"))
			})
		})

		When("fetching the orgs fails", func() {
			BeforeEach(func() {
				orgRepo.FetchOrgsReturns(nil, errors.New("boom!"))
				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnknownError(func() *httptest.ResponseRecorder { return rr })
		})
	})
})
