package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
)

const (
	rootURL  = "https://api.example.org"
	orgsBase = "/v3/organizations"
)

var _ = Describe("OrgHandler", func() {
	var (
		orgHandler *apis.OrgHandler
		orgRepo    *fake.OrgRepository
		now        time.Time
	)

	BeforeEach(func() {
		now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z

		orgRepo = new(fake.OrgRepository)
		decoderValidator, err := apis.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		orgHandler = apis.NewOrgHandler(*serverURL, orgRepo, decoderValidator)
		orgHandler.RegisterRoutes(router)
	})

	Describe("Create Org", func() {
		makePostRequest := func(requestBody string) {
			request, err := http.NewRequestWithContext(ctx, "POST", orgsBase, strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
			request.Header.Add(headers.Authorization, "Bearer my-token")

			router.ServeHTTP(rr, request)
		}

		BeforeEach(func() {
			orgRepo.CreateOrgStub = func(_ context.Context, info authorization.Info, message repositories.CreateOrgMessage) (repositories.OrgRecord, error) {
				record := repositories.OrgRecord{
					Name:        message.Name,
					GUID:        "t-h-e-o-r-g",
					Suspended:   message.Suspended,
					Labels:      message.Labels,
					Annotations: message.Annotations,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				return record, nil
			}
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
				_, info, orgRecord := orgRepo.CreateOrgArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(orgRecord.Name).To(Equal("the-org"))
				Expect(orgRecord.Suspended).To(BeFalse())
				Expect(orgRecord.Labels).To(BeEmpty())
				Expect(orgRecord.Annotations).To(BeEmpty())
			})
		})

		When("the org repo returns a uniqueness error", func() {
			BeforeEach(func() {
				var err error = &k8serrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReason(fmt.Sprintf(`{"code":%d}`, webhooks.DuplicateOrgNameError)),
					},
				}
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, err)
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Organization 'the-org' already exists.")
			})
		})

		When("the org repo returns another error", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, errors.New("boom"))
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
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
				_, info, orgRecord := orgRepo.CreateOrgArgsForCall(0)
				Expect(info).To(Equal(authInfo))
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

		When("authentication is invalid", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, authorization.InvalidAuthError{})
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns Unauthorized error", func() {
				Expect(rr.Result().StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(rr.Body.String()).To(MatchJSON(`{
                    "errors": [
                    {
                        "detail": "Invalid Auth Token",
                        "title": "CF-InvalidAuthToken",
                        "code": 1000
                    }
                    ]
                }`))
			})
		})

		When("authentication is not provided", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, authorization.NotAuthenticatedError{})
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns Unauthorized error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("user is not allowed to create an org", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, repositories.NewForbiddenError(repositories.OrgResourceType, errors.New("nope")))
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns an unauthorised error", func() {
				expectNotAuthorizedError()
			})
		})

		When("providing the repository fails", func() {
			BeforeEach(func() {
				orgRepo.CreateOrgReturns(repositories.OrgRecord{}, errors.New("boom!"))
				makePostRequest(`{"name": "the-org"}`)
			})

			It("returns Unknown error", func() {
				Expect(rr.Result().StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(rr.Body.String()).To(MatchJSON(`{
                    "errors": [
                    {
                        "title": "UnknownError",
                        "detail": "An unknown error occurred.",
                        "code": 10001
                    }
                    ]
                }`))
			})
		})
	})

	Describe("Listing Orgs", func() {
		var req *http.Request
		BeforeEach(func() {
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

			var err error
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, orgsBase, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add(headers.Authorization, "Bearer my-token")
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
				Expect(orgRepo.ListOrgsCallCount()).To(Equal(1))
				_, info, message := orgRepo.ListOrgsArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message.Names).To(BeEmpty())
			})

			It("renders the orgs response", func() {
				expectedBody := fmt.Sprintf(`
                {
                    "pagination": {
                        "total_results": 2,
                        "total_pages": 1,
                        "first": {
                            "href": "%[1]s/v3/organizations"
                        },
                        "last": {
                            "href": "%[1]s/v3/organizations"
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
				values := url.Values{
					"names": []string{"foo,bar"},
				}
				req.URL.RawQuery = values.Encode()

				router.ServeHTTP(rr, req)
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
				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				orgRepo.ListOrgsReturns(nil, authorization.InvalidAuthError{})
				router.ServeHTTP(rr, req)
			})

			It("returns Unauthorized error", func() {
				Expect(rr.Result().StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(rr.Body.String()).To(MatchJSON(`{
                    "errors": [
                    {
                        "detail": "Invalid Auth Token",
                        "title": "CF-InvalidAuthToken",
                        "code": 1000
                    }
                    ]
                }`))
			})
		})

		When("authentication is not provided", func() {
			BeforeEach(func() {
				orgRepo.ListOrgsReturns(nil, authorization.NotAuthenticatedError{})
				router.ServeHTTP(rr, req)
			})

			It("returns Unauthorized error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("providing the repository fails", func() {
			BeforeEach(func() {
				orgRepo.ListOrgsReturns(nil, errors.New("boom"))
				router.ServeHTTP(rr, req)
			})

			It("returns Unknown error", func() {
				Expect(rr.Result().StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(rr.Body.String()).To(MatchJSON(`{
                    "errors": [
                    {
                        "title": "UnknownError",
                        "detail": "An unknown error occurred.",
                        "code": 10001
                    }
                    ]
                }`))
			})
		})
	})

	Describe("Delete Org", func() {
		const (
			orgGUID = "orgGUID"
		)
		var (
			request *http.Request
			err     error
		)

		BeforeEach(func() {
			request, err = http.NewRequestWithContext(ctx, http.MethodDelete, orgsBase+"/"+orgGUID, nil)
			Expect(err).NotTo(HaveOccurred())
			request.Header.Add(headers.Authorization, "Bearer my-token")
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				router.ServeHTTP(rr, request)
			})
			It("responds with a 202 accepted response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			})

			It("responds with a job URL in a location header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/org.delete-"+orgGUID))
			})

			It("deletes the K8s record via the repository", func() {
				Expect(orgRepo.DeleteOrgCallCount()).To(Equal(1))
				_, info, message := orgRepo.DeleteOrgArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message).To(Equal(repositories.DeleteOrgMessage{
					GUID: orgGUID,
				}))
			})
		})

		When("deleting the org is not authorized", func() {
			BeforeEach(func() {
				orgRepo.DeleteOrgReturns(repositories.NewForbiddenError(repositories.OrgResourceType, nil))
				router.ServeHTTP(rr, request)
			})

			It("returns Unauthorized error", func() {
				expectNotAuthorizedError()
			})
		})

		When("invoking the delete org repository fails", func() {
			BeforeEach(func() {
				orgRepo.DeleteOrgReturns(errors.New("unknown-error"))
				router.ServeHTTP(rr, request)
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})

		When("the org doesn't exist", func() {
			BeforeEach(func() {
				orgRepo.DeleteOrgReturns(repositories.NewNotFoundError(repositories.OrgResourceType, nil))
				router.ServeHTTP(rr, request)
			})

			It("returns an error", func() {
				expectNotFoundError("Org not found")
			})
		})
	})
})
