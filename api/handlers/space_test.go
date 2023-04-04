package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Space", func() {
	const spacesBase = "/v3/spaces"

	var (
		now           time.Time
		apiHandler    *handlers.Space
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
		decoderValidator, err := handlers.NewDefaultDecoderValidator()
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
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

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
				Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/space.delete~"+spaceGUID))
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
		const (
			spaceGUID = "spaceGUID"
			orgGUID   = "orgGUID"
		)

		BeforeEach(func() {
			requestMethod = http.MethodPatch
			requestPath = spacesBase + "/" + spaceGUID
		})

		When("the space exists and is accessible and we patch the annotations and labels", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
					GUID:             spaceGUID,
					OrganizationGUID: orgGUID,
					Name:             "test-space",
				}, nil)

				spaceRepo.PatchSpaceMetadataReturns(repositories.SpaceRecord{
					GUID:             spaceGUID,
					Name:             "test-space",
					OrganizationGUID: orgGUID,
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

			It("returns status 200 OK", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})

			It("patches the space with the new labels and annotations", func() {
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(1))
				_, _, msg := spaceRepo.PatchSpaceMetadataArgsForCall(0)
				Expect(msg.GUID).To(Equal(spaceGUID))
				Expect(msg.OrgGUID).To(Equal(orgGUID))
				Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
				Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
				Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
				Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))
			})

			It("includes the labels and annotations in the response", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

				var jsonBody struct {
					Metadata struct {
						Annotations map[string]string `json:"annotations"`
						Labels      map[string]string `json:"labels"`
					} `json:"metadata"`
				}
				Expect(json.NewDecoder(rr.Body).Decode(&jsonBody)).To(Succeed())
				Expect(jsonBody.Metadata.Annotations).To(Equal(map[string]string{
					"hello":                       "there",
					"foo.example.com/lorem-ipsum": "Lorem ipsum.",
				}), "body = ", rr.Body.String())
				Expect(jsonBody.Metadata.Labels).To(Equal(map[string]string{
					"env":                           "production",
					"foo.example.com/my-identifier": "aruba",
				}))
			})
		})

		When("the user doesn't have permission to get the org", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(nil, repositories.SpaceResourceType))
				requestBody = `{
				  "metadata": {
					"labels": {
					  "env": "production"
					}
				  }
				}`
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.SpaceResourceType)
			})

			It("does not call patch", func() {
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the org errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
				requestBody = `{
				  "metadata": {
					"labels": {
					  "env": "production"
					}
				  }
				}`
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("does not call patch", func() {
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the org errors", func() {
			BeforeEach(func() {
				spaceRecord := repositories.SpaceRecord{
					GUID:             spaceGUID,
					OrganizationGUID: orgGUID,
					Name:             "test-space",
				}
				spaceRepo.GetSpaceReturns(spaceRecord, nil)
				spaceRepo.PatchSpaceMetadataReturns(repositories.SpaceRecord{}, errors.New("boom"))
				requestBody = `{
				  "metadata": {
					"labels": {
					  "env": "production"
					}
				  }
				}`
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
})
