package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Spaces", func() {
	Describe("Listing Spaces", func() {
		const spacesBase = "/v3/spaces"
		var (
			ctx          context.Context
			router       *mux.Router
			spaceHandler *apis.SpaceHandler
			spaceRepo    *fake.CFSpaceRepository
			req          *http.Request
			rr           *httptest.ResponseRecorder
			err          error
			now          time.Time
		)

		BeforeEach(func() {
			ctx = context.Background()
			spaceRepo = new(fake.CFSpaceRepository)

			now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z
			spaceRepo.FetchSpacesReturns([]repositories.SpaceRecord{
				{
					Name:             "alice",
					GUID:             "a-l-i-c-e",
					OrganizationGUID: "org-guid-1",
					CreatedAt:        now,
					UpdatedAt:        now,
				},
				{
					Name:             "bob",
					GUID:             "b-o-b",
					OrganizationGUID: "org-guid-2",
					CreatedAt:        now,
					UpdatedAt:        now,
				},
			}, nil)

			serverURL, err := url.Parse(rootURL)
			Expect(err).NotTo(HaveOccurred())
			spaceHandler = apis.NewSpaceHandler(spaceRepo, *serverURL)
			router = mux.NewRouter()
			spaceHandler.RegisterRoutes(router)

			rr = httptest.NewRecorder()
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, spacesBase, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a list of spaces", func() {
			router.ServeHTTP(rr, req)
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))

			Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
           "pagination": {
              "total_results": 2,
              "total_pages": 1,
              "first": {
                 "href": "%[1]s/v3/spaces?page=1"
              },
              "last": {
                 "href": "%[1]s/v3/spaces?page=1"
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
                    "metadata": {
                      "labels": {},
                      "annotations": {}
                    },
                    "relationships": {
                        "organization": {
                          "data": {
                            "guid": "org-guid-1"
                          }
                        }
                    },
                    "links": {
                        "self": {
                            "href": "%[1]s/v3/spaces/a-l-i-c-e"
                        },
                        "organization": {
                            "href": "%[1]s/v3/organizations/org-guid-1"
                        }
                    }
                },
                {
                    "guid": "b-o-b",
                    "name": "bob",
                    "created_at": "2021-09-17T15:23:10Z",
                    "updated_at": "2021-09-17T15:23:10Z",
                    "metadata": {
                      "labels": {},
                      "annotations": {}
                    },
                    "relationships": {
                        "organization": {
                          "data": {
                            "guid": "org-guid-2"
                          }
                        }
                    },
                    "links": {
                        "self": {
                            "href": "%[1]s/v3/spaces/b-o-b"
                        },
                        "organization": {
                            "href": "%[1]s/v3/organizations/org-guid-2"
                        }
                    }
                }
            ]
        }`, defaultServerURL)))

			Expect(spaceRepo.FetchSpacesCallCount()).To(Equal(1))
			_, organizationGUIDs, names := spaceRepo.FetchSpacesArgsForCall(0)
			Expect(organizationGUIDs).To(BeEmpty())
			Expect(names).To(BeEmpty())
		})

		When("fetching the spaces fails", func() {
			BeforeEach(func() {
				spaceRepo.FetchSpacesReturns(nil, errors.New("boom!"))
				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnknownError(func() *httptest.ResponseRecorder { return rr })
		})

		When("organization_guids are provided as a comma-separated list", func() {
			It("filters spaces by them", func() {
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, spacesBase+"?organization_guids=foo,,bar,", nil)
				Expect(err).NotTo(HaveOccurred())
				router.ServeHTTP(rr, req)

				Expect(spaceRepo.FetchSpacesCallCount()).To(Equal(1))
				_, organizationGUIDs, names := spaceRepo.FetchSpacesArgsForCall(0)
				Expect(organizationGUIDs).To(ConsistOf("foo", "bar"))
				Expect(names).To(BeEmpty())
			})
		})

		When("names are provided as a comma-separated list", func() {
			It("filters spaces by them", func() {
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, spacesBase+"?organization_guids=org1&names=foo,,bar,", nil)
				Expect(err).NotTo(HaveOccurred())
				router.ServeHTTP(rr, req)

				Expect(spaceRepo.FetchSpacesCallCount()).To(Equal(1))
				_, organizationGUIDs, names := spaceRepo.FetchSpacesArgsForCall(0)
				Expect(organizationGUIDs).To(ConsistOf("org1"))
				Expect(names).To(ConsistOf("foo", "bar"))
			})
		})
	})
})
