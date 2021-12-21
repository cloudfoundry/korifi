package apis_test

import (
	"errors"
	"fmt"
	"net/http"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("DomainHandler", func() {
	Describe("the GET /v3/domains endpoint", func() {
		const (
			testDomainGUID = "test-domain-guid"
		)

		var (
			domainRepo   *fake.CFDomainRepository
			domainRecord *repositories.DomainRecord
		)

		BeforeEach(func() {
			domainRepo = new(fake.CFDomainRepository)

			domainRecord = &repositories.DomainRecord{
				GUID:        testDomainGUID,
				Name:        "example.org",
				Labels:      nil,
				Annotations: nil,
				CreatedAt:   "2019-05-10T17:17:48Z",
				UpdatedAt:   "2019-05-10T17:17:48Z",
			}
			domainRepo.ListDomainsReturns([]repositories.DomainRecord{*domainRecord}, nil)

			domainHandler := NewDomainHandler(
				logf.Log.WithName("TestDomainHandler"),
				*serverURL,
				domainRepo,
			)
			domainHandler.RegisterRoutes(router)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				req, err := http.NewRequestWithContext(ctx, "GET", "/v3/domains", nil)
				Expect(err).NotTo(HaveOccurred())
				router.ServeHTTP(rr, req)
			})
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
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
		})

		When("no domain exists", func() {
			BeforeEach(func() {
				domainRepo.ListDomainsReturns([]repositories.DomainRecord{}, nil)
				req, err := http.NewRequestWithContext(ctx, "GET", "/v3/domains", nil)
				Expect(err).NotTo(HaveOccurred())
				router.ServeHTTP(rr, req)
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
				req, err := http.NewRequestWithContext(ctx, "GET", "/v3/domains", nil)
				Expect(err).NotTo(HaveOccurred())
				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				req, err := http.NewRequestWithContext(ctx, "GET", "/v3/domains?foo=bar", nil)
				Expect(err).NotTo(HaveOccurred())
				router.ServeHTTP(rr, req)
			})
			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'names'")
			})
		})
	})
})
