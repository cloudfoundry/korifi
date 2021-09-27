package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

const (
	rootURL  = "https://api.example.org"
	orgsBase = "/v3/organizations"
)

func TestOrg(t *testing.T) {
	spec.Run(t, "listing orgs", testListingOrgs, spec.Report(report.Terminal{}))
}

func testListingOrgs(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var (
		ctx        context.Context
		router     *mux.Router
		orgHandler *apis.OrgHandler
		orgRepo    *fake.CFOrgRepository
		req        *http.Request
		rr         *httptest.ResponseRecorder
		err        error
	)

	it.Before(func() {
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
		g.Expect(err).NotTo(HaveOccurred())
	})

	when("happy path", func() {
		it.Before(func() {
			router.ServeHTTP(rr, req)
		})

		it("returns 200", func() {
			g.Expect(rr.Result().StatusCode).To(Equal(http.StatusOK))
		})

		it("sets json content type", func() {
			g.Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
		})

		it("lists orgs using the repository", func() {
			g.Expect(orgRepo.FetchOrgsCallCount()).To(Equal(1))
			_, names := orgRepo.FetchOrgsArgsForCall(0)
			g.Expect(names).To(BeEmpty())
		})

		it("renders the orgs response", func() {
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
			g.Expect(rr.Body.String()).To(MatchJSON(expectedBody))
		})
	})

	when("names are specified", func() {
		it.Before(func() {
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, orgsBase+"?names=foo,bar", nil)
			g.Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		})

		it("filters by them", func() {
			g.Expect(orgRepo.FetchOrgsCallCount()).To(Equal(1))
			_, names := orgRepo.FetchOrgsArgsForCall(0)
			g.Expect(names).To(ConsistOf("foo", "bar"))
		})
	})

	when("fetching the orgs fails", func() {
		it.Before(func() {
			orgRepo.FetchOrgsReturns(nil, errors.New("boom!"))
			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, func() *httptest.ResponseRecorder { return rr })
	})
}
