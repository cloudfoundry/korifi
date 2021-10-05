package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	Describe("Listing Orgs", func() {
		var (
			ctx        context.Context
			router     *mux.Router
			orgHandler *apis.OrgHandler
			orgRepo    *fake.CFOrgRepository
			req        *http.Request
			rr         *httptest.ResponseRecorder
			err        error
		)

		BeforeEach(func() {
			ctx = context.Background()
			orgRepo = new(fake.CFOrgRepository)

			now := time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z
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

			orgHandler = apis.NewOrgHandler(orgRepo, rootURL)
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
