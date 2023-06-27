package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domain", func() {
	var (
		apiHandler       *handlers.Domain
		domainRepo       *fake.CFDomainRepository
		requestValidator *fake.RequestValidator
		req              *http.Request
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		domainRepo = new(fake.CFDomainRepository)
		apiHandler = handlers.NewDomain(
			*serverURL,
			requestValidator,
			domainRepo,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/domain", func() {
		var payload *payloads.DomainCreate

		BeforeEach(func() {
			payload = &payloads.DomainCreate{
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
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidateJSONPayloadStub(payload)

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
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/domains", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a domain", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

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

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "domain-guid"),
				MatchJSONPath("$.supported_protocols", ConsistOf("http")),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/domains/domain-guid"),
			)))
		})

		When("decoding the payload fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "oops"))
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

		It("returns the domain", func() {
			Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDomainGUID := domainRepo.GetDomainArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDomainGUID).To(Equal("domain-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "domain-guid"),
				MatchJSONPath("$.supported_protocols", ConsistOf("http")),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/domains/domain-guid"),
			)))
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
				expectNotFoundError("CFDomain")
			})
		})
	})

	Describe("PATCH /v3/domains/:guid", func() {
		var payload *payloads.DomainUpdate

		BeforeEach(func() {
			payload = &payloads.DomainUpdate{
				Metadata: payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"bar": tools.PtrTo("baz"),
					},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidateJSONPayloadStub(payload)

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
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/domains/my-domain", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("updates the domain", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(domainRepo.UpdateDomainCallCount()).To(Equal(1))
			_, _, updateMessage := domainRepo.UpdateDomainArgsForCall(0)
			Expect(updateMessage).To(Equal(repositories.UpdateDomainMessage{
				GUID: "my-domain",
				MetadataPatch: repositories.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "domain-guid"),
				MatchJSONPath("$.supported_protocols", ConsistOf("http")),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/domains/domain-guid"),
			)))
		})

		When("decoding the payload fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "oops"))
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
				expectNotFoundError("CFDomain")
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

			payload := &payloads.DomainList{Names: "bob,alice"}
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(payload)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/domains", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the list of domains", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL).To(Equal(req.URL))

			Expect(domainRepo.ListDomainsCallCount()).To(Equal(1))
			_, _, listMessage := domainRepo.ListDomainsArgsForCall(0)
			Expect(listMessage).To(Equal(repositories.ListDomainsMessage{
				Names: []string{"bob", "alice"},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/domains"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].guid", "test-domain-guid"),
				MatchJSONPath("$.resources[0].supported_protocols", ConsistOf("http")),
			)))
		})

		When("no domain exists", func() {
			BeforeEach(func() {
				domainRepo.ListDomainsReturns([]repositories.DomainRecord{}, nil)
			})

			It("returns status 200 OK", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.pagination.total_results", BeZero()),
					MatchJSONPath("$.resources", BeEmpty()),
				)))
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

		It("deletes the domain", func() {
			Expect(domainRepo.DeleteDomainCallCount()).To(Equal(1))
			_, _, deletedDomainGUID := domainRepo.DeleteDomainArgsForCall(0)
			Expect(deletedDomainGUID).To(Equal("my-domain"))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
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
				expectNotFoundError("CFDomain")
			})
		})
	})
})
