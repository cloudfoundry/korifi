package handlers_test

import (
	"errors"
	"net/http"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Org", func() {
	var (
		apiHandler *handlers.Org
		orgRepo    *fake.OrgRepository
		now        string
		domainRepo *fake.CFDomainRepository
	)

	BeforeEach(func() {
		now = time.Unix(1631892190, 0).UTC().Format(time.RFC3339) // 2021-09-17T15:23:10Z

		orgRepo = new(fake.OrgRepository)
		domainRepo = new(fake.CFDomainRepository)
		decoderValidator, err := handlers.NewGoPlaygroundValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler = handlers.NewOrg(*serverURL, orgRepo, domainRepo, decoderValidator, time.Hour)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("Create Org", func() {
		var req string

		BeforeEach(func() {
			req = `{
				"name": "the-org",
				"suspended": true,
				"metadata": {
					"labels": {"foo": "bar"},
					"annotations": {"bar": "baz"}
				}
			}`

			orgRepo.CreateOrgReturns(repositories.OrgRecord{
				Name:      "new-org",
				GUID:      "org-guid",
				Suspended: false,
				Labels: map[string]string{
					"label-key": "label-val",
				},
				Annotations: map[string]string{
					"annotation-key": "annotation-val",
				},
				CreatedAt: "then",
				UpdatedAt: "later",
			}, nil)
		})

		JustBeforeEach(func() {
			request, err := http.NewRequestWithContext(ctx, "POST", "/v3/organizations", strings.NewReader(req))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, request)
		})

		It("creates the org", func() {
			Expect(orgRepo.CreateOrgCallCount()).To(Equal(1))
			_, info, orgRecord := orgRepo.CreateOrgArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(orgRecord.Name).To(Equal("the-org"))
			Expect(orgRecord.Suspended).To(BeTrue())
			Expect(orgRecord.Labels).To(And(HaveLen(1), HaveKeyWithValue("foo", "bar")))
			Expect(orgRecord.Annotations).To(And(HaveLen(1), HaveKeyWithValue("bar", "baz")))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "org-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/organizations/org-guid"),
			)))
		})

		When("the org repo returns an error", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, errors.New("boom"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				req = `{`
			})

			It("returns a status 400 with appropriate error JSON", func() {
				expectBadRequestError()
			})
		})

		When("the request body has an unknown field", func() {
			BeforeEach(func() {
				req = `{"description" : "Invalid Request"}`
			})

			It("returns a status 422 with appropriate error JSON", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "description"`)
			})
		})

		When("the request body is invalid with invalid app name", func() {
			BeforeEach(func() {
				req = `{"name": 12345}`
			})

			It("returns a status 422 with appropriate error JSON", func() {
				expectUnprocessableEntityError("Name must be a string")
			})
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
				req = `{"metadata": {"labels": {"foo": "bar"}}}`
			})

			It("returns a status 422 with appropriate error message json", func() {
				expectUnprocessableEntityError("Name is a required field")
			})
		})
	})

	Describe("Listing Orgs", func() {
		var path string

		BeforeEach(func() {
			path = "/v3/organizations"
			orgRepo.ListOrgsReturns([]repositories.OrgRecord{
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
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the org list", func() {
			Expect(orgRepo.ListOrgsCallCount()).To(Equal(1))
			_, info, message := orgRepo.ListOrgsArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message.Names).To(BeEmpty())

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/organizations"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "a-l-i-c-e"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/organizations/a-l-i-c-e"),
				MatchJSONPath("$.resources[1].guid", "b-o-b"),
			)))
		})

		When("names are specified", func() {
			BeforeEach(func() {
				path += "?names=foo,bar"
			})

			It("filters by them", func() {
				Expect(orgRepo.ListOrgsCallCount()).To(Equal(1))
				_, info, message := orgRepo.ListOrgsArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message.Names).To(ConsistOf("foo", "bar"))
			})
		})

		When("fetching the orgs fails", func() {
			BeforeEach(func() {
				orgRepo.ListOrgsReturns(nil, errors.New("boom!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("Updating Orgs", func() {
		var requestBody string

		JustBeforeEach(func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/v3/organizations/org-guid", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, request)
		})

		BeforeEach(func() {
			orgRepo.GetOrgReturns(repositories.OrgRecord{
				GUID: "org-guid",
				Name: "test-org",
			}, nil)

			orgRepo.PatchOrgMetadataReturns(repositories.OrgRecord{
				GUID: "org-guid",
				Name: "test-org",
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

		It("patches the org", func() {
			Expect(orgRepo.PatchOrgMetadataCallCount()).To(Equal(1))
			_, _, msg := orgRepo.PatchOrgMetadataArgsForCall(0)
			Expect(msg.GUID).To(Equal("org-guid"))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.metadata.annotations", map[string]any{
					"hello":                       "there",
					"foo.example.com/lorem-ipsum": "Lorem ipsum.",
				}),
				MatchJSONPath("$.metadata.labels", map[string]any{
					"env":                           "production",
					"foo.example.com/my-identifier": "aruba",
				}),
			)))
		})

		When("the user doesn't have permission to get the org", func() {
			BeforeEach(func() {
				orgRepo.GetOrgReturns(repositories.OrgRecord{}, apierrors.NewForbiddenError(nil, repositories.OrgResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.OrgResourceType)
			})

			It("does not call patch", func() {
				Expect(orgRepo.PatchOrgMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the org errors", func() {
			BeforeEach(func() {
				orgRepo.GetOrgReturns(repositories.OrgRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("does not call patch", func() {
				Expect(orgRepo.PatchOrgMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the org errors", func() {
			BeforeEach(func() {
				orgRepo.PatchOrgMetadataReturns(repositories.OrgRecord{}, errors.New("boom"))
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
							"annotations": {
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
	})

	Describe("Delete Org", func() {
		JustBeforeEach(func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodDelete, "/v3/organizations/org-guid", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, request)
		})

		It("deletes the org", func() {
			Expect(orgRepo.DeleteOrgCallCount()).To(Equal(1))
			_, info, message := orgRepo.DeleteOrgArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message).To(Equal(repositories.DeleteOrgMessage{
				GUID: "org-guid",
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/org.delete~org-guid"))
		})

		When("invoking the delete org repository yields a forbidden error", func() {
			BeforeEach(func() {
				orgRepo.DeleteOrgReturns(apierrors.NewForbiddenError(errors.New("boom"), repositories.OrgResourceType))
			})

			It("returns NotFound error", func() {
				expectNotFoundError(repositories.OrgResourceType)
			})
		})

		When("invoking the delete org repository fails", func() {
			BeforeEach(func() {
				orgRepo.DeleteOrgReturns(errors.New("unknown-error"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("List Domains", func() {
		var (
			domainRecord *repositories.DomainRecord
			requestURL   string
		)

		BeforeEach(func() {
			domainRecord = &repositories.DomainRecord{
				GUID:        "domain-guid",
				Name:        "example.org",
				Labels:      nil,
				Annotations: nil,
				CreatedAt:   "2019-05-10T17:17:48Z",
				UpdatedAt:   "2019-05-10T17:17:48Z",
			}
			domainRepo.ListDomainsReturns([]repositories.DomainRecord{*domainRecord}, nil)
			requestURL = "/v3/organizations/org-guid/domains"
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("lists the org domains", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/organizations/org-guid/domains"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].guid", "domain-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/domains/domain-guid"),
			)))
		})

		When("getting the Org fails", func() {
			BeforeEach(func() {
				orgRepo.GetOrgReturns(repositories.OrgRecord{}, errors.New("failed to get org"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})

		When("getting the Org is forbidden", func() {
			BeforeEach(func() {
				orgRepo.GetOrgReturns(repositories.OrgRecord{}, apierrors.NewForbiddenError(errors.New("boom"), repositories.OrgResourceType))
			})

			It("returns an NotFound error", func() {
				expectNotFoundError(repositories.OrgResourceType)
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

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				requestURL += "?foo=bar"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'names'")
			})
		})
	})
})
