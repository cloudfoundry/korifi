package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Space", func() {
	var (
		apiHandler    *handlers.Space
		spaceRepo     *fake.SpaceRepository
		requestMethod string
		requestBody   string
		requestPath   string
	)

	BeforeEach(func() {
		requestBody = ""
		requestPath = "/v3/spaces"

		spaceRepo = new(fake.SpaceRepository)
		spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
			Name:             "the-space",
			GUID:             "the-space-guid",
			OrganizationGUID: "the-org-guid",
		}, nil)

		decoderValidator, err := handlers.NewGoPlaygroundValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler = handlers.NewSpace(
			*serverURL,
			spaceRepo,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("Create Space", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPost

			spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{
				Name:             "the-space",
				GUID:             "the-space-guid",
				OrganizationGUID: "the-org-guid",
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

		It("creates the space", func() {
			Expect(spaceRepo.CreateSpaceCallCount()).To(Equal(1))
			_, info, spaceRecord := spaceRepo.CreateSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(spaceRecord.Name).To(Equal("the-space"))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-space-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/spaces/the-space-guid"),
			)))
		})

		When("the parent org does not exist (and the repo returns a not found error)", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, apierrors.NewNotFoundError(errors.New("nope"), repositories.OrgResourceType))
			})

			It("returns an unauthorised error", func() {
				expectUnprocessableEntityError("Invalid organization. Ensure the organization exists and you have access to it.")
			})
		})

		When("creating the space fails", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, errors.New("nope"))
			})

			It("returns an error", func() {
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
	})

	Describe("Listing Spaces", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
				{
					Name:             "test-space-1",
					GUID:             "test-space-1-guid",
					OrganizationGUID: "test-org-1-guid",
				},
				{
					Name:             "test-space-2",
					GUID:             "test-space-2-guid",
					OrganizationGUID: "test-org-2-guid",
				},
			}, nil)
		})

		It("returns a list of spaces", func() {
			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
			_, info, message := spaceRepo.ListSpacesArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message.OrganizationGUIDs).To(BeEmpty())
			Expect(message.Names).To(BeEmpty())

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/spaces"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "test-space-1-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/spaces/test-space-1-guid"),
				MatchJSONPath("$.resources[1].guid", "test-space-2-guid"),
			)))
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
				requestPath += "?organization_guids=foo,,bar,"
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
				requestPath += "?organization_guids=org1&names=foo,,bar,"
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
		BeforeEach(func() {
			requestMethod = http.MethodDelete
			requestPath += "/the-space-guid"
		})

		It("deletes the space", func() {
			Expect(spaceRepo.GetSpaceCallCount()).To(Equal(1))
			_, info, actualSpaceGUID := spaceRepo.GetSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(actualSpaceGUID).To(Equal("the-space-guid"))

			Expect(spaceRepo.DeleteSpaceCallCount()).To(Equal(1))
			_, info, message := spaceRepo.DeleteSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message).To(Equal(repositories.DeleteSpaceMessage{
				GUID:             "the-space-guid",
				OrganizationGUID: "the-org-guid",
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/space.delete~the-space-guid"))
		})

		When("fetching the space errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
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

	Describe("Updating Space", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPatch
			requestPath += "/the-space-guid"

			spaceRepo.PatchSpaceMetadataReturns(repositories.SpaceRecord{
				Name:             "the-space",
				GUID:             "the-space-guid",
				OrganizationGUID: "the-org-guid",
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

		It("updates the space", func() {
			Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(1))
			_, _, msg := spaceRepo.PatchSpaceMetadataArgsForCall(0)
			Expect(msg.GUID).To(Equal("the-space-guid"))
			Expect(msg.OrgGUID).To(Equal("the-org-guid"))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-space-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/spaces/the-space-guid"),
			)))
		})

		When("the user doesn't have permission to get the org", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(nil, repositories.SpaceResourceType))
			})

			It("returns a not found error and does not try updating the space", func() {
				expectNotFoundError(repositories.SpaceResourceType)
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the org errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns an error and does not try updating the space", func() {
				expectUnknownError()
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the org errors", func() {
			BeforeEach(func() {
				spaceRepo.PatchSpaceMetadataReturns(repositories.SpaceRecord{}, errors.New("boom"))
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
						  "cloudfoundry.org/test": "there"
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
							  "korifi.cloudfoundry.org/test": "there"
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
})
