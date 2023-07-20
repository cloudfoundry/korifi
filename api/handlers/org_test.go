package handlers_test

import (
	"errors"
	"net/http"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Org", func() {
	var (
		apiHandler       *handlers.Org
		orgRepo          *fake.CFOrgRepository
		now              time.Time
		domainRepo       *fake.CFDomainRepository
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z

		orgRepo = new(fake.CFOrgRepository)
		domainRepo = new(fake.CFDomainRepository)
		requestValidator = new(fake.RequestValidator)

		apiHandler = handlers.NewOrg(*serverURL, orgRepo, domainRepo, requestValidator, time.Hour, "the-default.domain")
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("Create Org", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.OrgCreate{
				Name:      "the-org",
				Suspended: true,
				Metadata: payloads.Metadata{
					Annotations: map[string]string{
						"bar": "baz",
					},
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			})

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
				CreatedAt: time.UnixMilli(1),
				UpdatedAt: tools.PtrTo(time.UnixMilli(2)),
			}, nil)
		})

		JustBeforeEach(func() {
			request, err := http.NewRequestWithContext(ctx, "POST", "/v3/organizations", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, request)
		})

		It("creates the org", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

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

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("Listing Orgs", func() {
		var path string

		BeforeEach(func() {
			path = "/v3/organizations?names=a,b"
			orgRepo.ListOrgsReturns([]repositories.OrgRecord{
				{
					Name:      "alice",
					GUID:      "a-l-i-c-e",
					CreatedAt: now,
					UpdatedAt: &now,
				},
				{
					Name:      "bob",
					GUID:      "b-o-b",
					CreatedAt: now,
					UpdatedAt: &now,
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the org list", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(Equal(path))

			Expect(orgRepo.ListOrgsCallCount()).To(Equal(1))
			_, info, message := orgRepo.ListOrgsArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message.Names).To(BeEmpty())

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/organizations?names=a,b"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "a-l-i-c-e"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/organizations/a-l-i-c-e"),
				MatchJSONPath("$.resources[1].guid", "b-o-b"),
			)))
		})

		When("names are specified", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(
					&payloads.OrgList{
						Names: "foo,bar",
					},
				)
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

			requestBody = "the-json-body"

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.OrgPatch{
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{
						"hello":                       tools.PtrTo("there"),
						"foo.example.com/lorem-ipsum": tools.PtrTo("Lorem ipsum."),
					},
					Labels: map[string]*string{
						"env":                           tools.PtrTo("production"),
						"foo.example.com/my-identifier": tools.PtrTo("aruba"),
					},
				},
			})
		})

		It("patches the org", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

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

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
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
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix(requestURL))

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

		When("the request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("oops"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("Get the default domain", func() {
		BeforeEach(func() {
			domainRepo.GetDomainByNameReturns(repositories.DomainRecord{
				GUID: "the-default-domain-guid",
				Name: "the-default.domain",
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/organizations/org-guid/domains/default", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the configured default domain", func() {
			Expect(orgRepo.GetOrgCallCount()).To(Equal(1))
			_, info, orgGUID := orgRepo.GetOrgArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(orgGUID).To(Equal("org-guid"))

			Expect(domainRepo.GetDomainByNameCallCount()).To(Equal(1))
			_, info, defaultDomainName := domainRepo.GetDomainByNameArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(defaultDomainName).To(Equal("the-default.domain"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-default-domain-guid"),
				MatchJSONPath("$.name", "the-default.domain"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/domains/the-default-domain-guid"),
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

		When("getting the Domain fails", func() {
			BeforeEach(func() {
				domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("failed to get domain"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})

		When("getting the Domain is forbidden", func() {
			BeforeEach(func() {
				domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, apierrors.NewForbiddenError(errors.New("boom"), repositories.DomainResourceType))
			})

			It("returns an NotFound error", func() {
				expectNotFoundError(repositories.DomainResourceType)
			})
		})
	})

	Describe("get an org", func() {
		BeforeEach(func() {
			orgRepo.GetOrgReturns(repositories.OrgRecord{
				Name: "org-name",
				GUID: "org-guid",
			}, nil)
		})

		JustBeforeEach(func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, "/v3/organizations/org-guid", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, request)
		})

		It("gets the org", func() {
			Expect(orgRepo.GetOrgCallCount()).To(Equal(1))
			_, info, actualOrgGUID := orgRepo.GetOrgArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(actualOrgGUID).To(Equal("org-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "org-guid"),
				MatchJSONPath("$.name", "org-name"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/organizations/org-guid"),
			)))
		})

		When("getting the org is forbidden", func() {
			BeforeEach(func() {
				orgRepo.GetOrgReturns(repositories.OrgRecord{}, apierrors.NewForbiddenError(nil, repositories.OrgResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.OrgResourceType)
			})
		})

		When("getting the org fails", func() {
			BeforeEach(func() {
				orgRepo.GetOrgReturns(repositories.OrgRecord{}, errors.New("get-org-err"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})
})
