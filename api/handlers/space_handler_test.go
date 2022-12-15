package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	apis "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Spaces", func() {
	const spacesBase = "/v3/spaces"

	var (
		now                time.Time
		spaceHandler       *apis.SpaceHandler
		spaceRepo          *fake.SpaceRepository
		manifestApplier    *fake.ManifestApplier
		requestMethod      string
		requestBody        string
		requestPath        string
		requestContentType string
	)

	BeforeEach(func() {
		now = time.Unix(1631892190, 0) // 2021-09-17T15:23:10Z
		requestBody = ""
		requestContentType = ""
		requestPath = spacesBase
		spaceRepo = new(fake.SpaceRepository)
		manifestApplier = new(fake.ManifestApplier)
		decoderValidator, err := apis.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		spaceHandler = apis.NewSpaceHandler(
			*serverURL,
			spaceRepo,
			manifestApplier,
			decoderValidator,
		)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Add("Content-type", requestContentType)
		spaceHandler.ServeHTTP(rr, req)
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
				Expect(rr.Code).To(Equal(http.StatusOK))
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
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

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

	Describe("POST /v3/spaces/{spaceGUID}/actions/apply_manifest", func() {
		BeforeEach(func() {
			requestMethod = "POST"
			requestPath = "/v3/spaces/" + spaceGUID + "/actions/apply_manifest"
			requestContentType = "application/x-yaml"
		})

		When("the manifest is valid", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                  default-route: true
                  memory: 128M
                  disk_quota: 256M
                  processes:
                  - type: web
                    command: start-web.sh
                    disk_quota: 512M
                    health-check-http-endpoint: /healthcheck
                    health-check-invocation-timeout: 5
                    health-check-type: http
                    instances: 1
                    memory: 256M
                    timeout: 10
                `
			})

			It("returns 202 with a Location header", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))

				Expect(rr).To(HaveHTTPHeaderWithValue("Location", ContainSubstring("space.apply_manifest~"+spaceGUID)))
			})

			It("calls applyManifestAction and passes it the authInfo from the context", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ := manifestApplier.ApplyArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("passes the parsed manifest to the action", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
				_, _, _, payload := manifestApplier.ApplyArgsForCall(0)

				Expect(payload.Applications).To(HaveLen(1))
				Expect(payload.Applications[0].Name).To(Equal("app1"))
				Expect(payload.Applications[0].DefaultRoute).To(BeTrue())
				Expect(payload.Applications[0].Memory).To(PointTo(Equal("128M")))
				Expect(payload.Applications[0].DiskQuota).To(PointTo(Equal("256M")))

				Expect(payload.Applications[0].Processes).To(HaveLen(1))
				Expect(payload.Applications[0].Processes[0].Type).To(Equal("web"))
				Expect(payload.Applications[0].Processes[0].Command).NotTo(BeNil())
				Expect(payload.Applications[0].Processes[0].Command).To(PointTo(Equal("start-web.sh")))
				Expect(payload.Applications[0].Processes[0].DiskQuota).To(PointTo(Equal("512M")))
				Expect(payload.Applications[0].Processes[0].HealthCheckHTTPEndpoint).To(PointTo(Equal("/healthcheck")))
				Expect(payload.Applications[0].Processes[0].HealthCheckInvocationTimeout).To(PointTo(Equal(int64(5))))
				Expect(payload.Applications[0].Processes[0].HealthCheckType).To(PointTo(Equal("http")))
				Expect(payload.Applications[0].Processes[0].Instances).To(PointTo(Equal(1)))
				Expect(payload.Applications[0].Processes[0].Memory).To(PointTo(Equal("256M")))
				Expect(payload.Applications[0].Processes[0].Timeout).To(PointTo(Equal(int64(10))))
			})
		})

		When("the manifest contains unsupported fields", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                  metadata:
                    annotations:
                      contact: "bob@example.com jane@example.com"
                    labels:
                      sensitive: true
                `
			})

			It("calls applyManifestAction and passes it the authInfo from the context", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ := manifestApplier.ApplyArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})
		})

		When("the manifest contains multiple apps", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                - name: app2
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Applications must contain at maximum 1 item")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("the application name is missing", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - {}
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Name is a required field")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("the application memory is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: test-app
                  memory: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].Memory' Error:Field validation for 'Memory' failed on the 'megabytestring' tag")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("the application route is invalid", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: my-app
                  routes:
                  - route: not-a-uri?
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError(`"not-a-uri?" is not a valid route URI`)
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("the application process instance count is negative", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    instances: -1
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Instances must be 0 or greater")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("the application process disk is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    disk_quota: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].Processes[0].DiskQuota' Error:Field validation for 'DiskQuota' failed on the 'megabytestring' tag")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("the application process memory is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    memory: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].Processes[0].Memory' Error:Field validation for 'Memory' failed on the 'megabytestring' tag")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("a process's health-check-type is invalid", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    health-check-type: bogus-type
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("HealthCheckType must be one of [none process port http]")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("a process's healthcheck timeout is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                applications:
                - name: test-app
                  processes:
                  - type: web
                    timeout: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Timeout must be 1 or greater")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("a process's healthcheck invocation timeout is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                applications:
                - name: test-app
                  processes:
                  - type: web
                    health-check-invocation-timeout: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("HealthCheckInvocationTimeout must be 1 or greater")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("app healthcheck timeout is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                applications:
                - name: test-app
                  timeout: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Timeout must be 1 or greater")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("app healthcheck invocation timeout is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = `---
                applications:
                - name: test-app
                  health-check-invocation-timeout: 0
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("HealthCheckInvocationTimeout must be 1 or greater")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(0))
			})
		})

		When("applying the manifest errors", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                `
				manifestApplier.ApplyReturns(errors.New("boom"))
			})

			It("respond with Unknown Error", func() {
				expectUnknownError()
			})
		})

		When("applying the manifest errors with NotFoundErr", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                `
				manifestApplier.ApplyReturns(apierrors.NewNotFoundError(errors.New("can't find"), repositories.DomainResourceType))
			})

			It("respond with NotFoundErr", func() {
				expectNotFoundError("Domain")
			})
		})

		When("The random-route and default-route flags are both set", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                  default-route: true
                  random-route: true
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].defaultRoute' Error:Field validation for 'defaultRoute' failed on the 'Random-route and Default-route may not be used together' tag")
			})
		})

		When("app disk-quota and app disk_quota are both set", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                  disk-quota: 128M
                  disk_quota: 128M
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Cannot set both 'disk-quota' and 'disk_quota' in manifest")
			})
		})

		When("process disk-quota and process disk_quota are both set", func() {
			BeforeEach(func() {
				requestBody = `---
                version: 1
                applications:
                - name: app1
                  processes:
                  - type: foo
                    disk-quota: 128M
                    disk_quota: 128M
                `
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Cannot set both 'disk-quota' and 'disk_quota' in manifest")
			})
		})
	})

	Describe("POST /v3/spaces/{spaceGUID}/manifest_diff", func() {
		BeforeEach(func() {
			requestMethod = "POST"
			requestPath = "/v3/spaces/" + spaceGUID + "/manifest_diff"
			requestContentType = "application/x-yaml"
			requestBody = `---
					version: 1
					applications:
					  - name: app1
				`
		})

		When("the space exists", func() {
			It("returns 202 with an empty diff", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                	"diff": []
            	}`)))
			})
		})

		When("getting the space errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("foo"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("getting the space is forbidden", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(errors.New("foo"), repositories.SpaceResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Space")
			})
		})
	})
})
