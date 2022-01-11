package apis_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
)

var _ = Describe("Spaces", func() {
	const spacesBase = "/v3/spaces"
	const registryCredentialsSecretName = "image-registry-credentials"

	var (
		now           time.Time
		spaceHandler  *apis.SpaceHandler
		spaceRepo     *fake.SpaceRepository
		requestMethod string
		requestBody   string
		requestPath   string
	)

	BeforeEach(func() {
		now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z
		requestBody = ""
		requestPath = spacesBase
		spaceRepo = new(fake.SpaceRepository)
		spaceHandler = apis.NewSpaceHandler(*serverURL, registryCredentialsSecretName, spaceRepo)
		spaceHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	})

	Describe("Create Space", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPost

			spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{
				Name:             "the-space",
				GUID:             "t-h-e-s-p-a-c-e",
				OrganizationGUID: "the-org",
				CreatedAt:        now,
				UpdatedAt:        now,
			}, nil)

			requestBody = `{
                "name": "the-space",
                "relationships": {
                    "organization": {
                        "data": {
                            "guid": "[org-guid]"
                        }
                    }
                }
            }`
		})

		It("returns 201 with appropriate success JSON", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(fmt.Sprintf(`{
                "guid": "t-h-e-s-p-a-c-e",
                "name": "the-space",
                "created_at": "2021-09-17T15:23:10Z",
                "updated_at": "2021-09-17T15:23:10Z",
                "metadata": {
                    "labels": {},
                    "annotations": {}
                },
                "relationships": {
                    "organization": {
                        "data": {
                            "guid": "the-org"
                        }
                    }
                },
                "links": {
                    "self": {
                        "href": "%[1]s/v3/spaces/t-h-e-s-p-a-c-e"
                    },
                    "organization": {
                        "href": "%[1]s/v3/organizations/the-org"
                    }
                }
            }`, defaultServerURL))))
		})

		It("uses the space repo to create the space", func() {
			Expect(spaceRepo.CreateSpaceCallCount()).To(Equal(1))
			_, info, spaceRecord := spaceRepo.CreateSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(spaceRecord.Name).To(Equal("the-space"))
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, authorization.InvalidAuthError{})
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
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, authorization.NotAuthenticatedError{})
			})

			It("returns Unauthorized error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("providing the space repository fails", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, errors.New("space-repo-provisioning-failed"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})

		When("a field in the request has invalid value", func() {
			BeforeEach(func() {
				requestBody = `{
                    "name": 123,
                    "relationships": {
                        "organization": {
                            "data": {
                                "guid": "[org-guid]"
                            }
                        }
                    }
                }`
			})

			It("returns a bad request error", func() {
				expectUnprocessableEntityError("Name must be a string")
			})
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				requestBody = `{"definitely not valid json`
			})

			It("returns a bad request error", func() {
				expectBadRequestError()
			})
		})

		When("when a required field is not provided", func() {
			BeforeEach(func() {
				requestBody = `{
                    "name": "the-name",
                    "relationships": {
                    }
                }`
			})

			It("returns a bad request error", func() {
				expectUnprocessableEntityError("Data is a required field")
			})
		})

		When("the space repo returns a uniqueness error", func() {
			BeforeEach(func() {
				var err error = &k8serrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReason(fmt.Sprintf(`{"code":%d}`, workloads.DuplicateSpaceNameError)),
					},
				}
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, err)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Space 'the-space' already exists.")
			})
		})

		When("the space repo returns another error", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("Listing Spaces", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
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
		})

		It("returns a list of spaces", func() {
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))

			Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
                "pagination": {
                    "total_results": 2,
                    "total_pages": 1,
                    "first": {
                        "href": "%[1]s/v3/spaces"
                    },
                    "last": {
                        "href": "%[1]s/v3/spaces"
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

			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
			_, info, message := spaceRepo.ListSpacesArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message.OrganizationGUIDs).To(BeEmpty())
			Expect(message.Names).To(BeEmpty())
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, authorization.InvalidAuthError{})
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
				spaceRepo.ListSpacesReturns(nil, authorization.NotAuthenticatedError{})
			})

			It("returns Unauthorized error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("providing the space repository fails", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("space-repo-provisioning-failed"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})

		When("fetching the spaces fails", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("boom!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("organization_guids are provided as a comma-separated list", func() {
			BeforeEach(func() {
				requestPath = spacesBase + "?organization_guids=foo,,bar,"
			})

			It("filters spaces by them", func() {
				Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
				_, info, message := spaceRepo.ListSpacesArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message.OrganizationGUIDs).To(ConsistOf("foo", "bar"))
				Expect(message.Names).To(BeEmpty())
			})
		})

		When("names are provided as a comma-separated list", func() {
			BeforeEach(func() {
				requestPath = spacesBase + "?organization_guids=org1&names=foo,,bar,"
			})

			It("filters spaces by them", func() {
				Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
				_, info, message := spaceRepo.ListSpacesArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message.OrganizationGUIDs).To(ConsistOf("org1"))
				Expect(message.Names).To(ConsistOf("foo", "bar"))
			})
		})
	})

	Describe("Deleting a Space", func() {
		const (
			spaceGUID = "spaceGUID"
			orgGUID   = "orgGUID"
		)

		BeforeEach(func() {
			requestMethod = http.MethodDelete
			requestPath = spacesBase + "/" + spaceGUID

			space := repositories.SpaceRecord{
				GUID:             spaceGUID,
				OrganizationGUID: orgGUID,
			}

			spaceRepo.GetSpaceReturns(space, nil)
			spaceRepo.DeleteSpaceReturns(nil)
		})

		When("on the happy path", func() {
			It("responds with a 202 accepted response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			})

			It("responds with a job URL in a location header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/space.delete-"+spaceGUID))
			})

			It("fetches the right space", func() {
				Expect(spaceRepo.GetSpaceCallCount()).To(Equal(1))
				_, info, actualSpaceGUID := spaceRepo.GetSpaceArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(actualSpaceGUID).To(Equal(spaceGUID))
			})

			It("deletes the K8s record via the repository", func() {
				Expect(spaceRepo.DeleteSpaceCallCount()).To(Equal(1))
				_, info, message := spaceRepo.DeleteSpaceArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message).To(Equal(repositories.DeleteSpaceMessage{
					GUID:             spaceGUID,
					OrganizationGUID: orgGUID,
				}))
			})
		})

		When("authentication is invalid", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, authorization.InvalidAuthError{})
			})

			It("returns Unauthorized error", func() {
				Expect(rr.Result().StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", jsonHeader))
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
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, authorization.NotAuthenticatedError{})
			})

			It("returns Unauthorized error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("providing the space repository fails", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("space-repo-provisioning-failed"))
			})

			It("returns unknown error", func() {
				expectUnknownError()
			})
		})

		When("the space doesn't exist", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, repositories.NotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("Space not found")
			})
		})

		When("fetching the space errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("deleting the space is not authorized", func() {
			BeforeEach(func() {
				spaceRepo.DeleteSpaceReturns(authorization.InvalidAuthError{Err: errors.New("boom")})
			})

			It("returns a 403 error", func() {
				expectUnauthorizedError()
			})
		})

		When("deleting the space errors", func() {
			BeforeEach(func() {
				spaceRepo.DeleteSpaceReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
