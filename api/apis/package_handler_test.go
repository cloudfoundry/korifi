package apis_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	testPackageHandlerLoggerName = "TestPackageHandler"
)

var _ = Describe("PackageHandler", func() {
	var (
		packageRepo                *fake.CFPackageRepository
		appRepo                    *fake.CFAppRepository
		dropletRepo                *fake.CFDropletRepository
		uploadImageSource          *fake.SourceImageUploader
		buildRegistryAuth          *fake.RegistryAuthBuilder
		packageRegistryBase        string
		packageImagePullSecretName string
	)

	BeforeEach(func() {
		packageRepo = new(fake.CFPackageRepository)
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		uploadImageSource = new(fake.SourceImageUploader)
		buildRegistryAuth = new(fake.RegistryAuthBuilder)
		packageRegistryBase = ""
		packageImagePullSecretName = ""
	})

	JustBeforeEach(func() {
		apiHandler := NewPackageHandler(
			logf.Log.WithName(testPackageHandlerLoggerName),
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			uploadImageSource.Spy,
			buildRegistryAuth.Spy,
			packageRegistryBase,
			packageImagePullSecretName,
		)

		apiHandler.RegisterRoutes(router)
	})

	Describe("the GET /v3/packages/:guid endpoint", func() {
		const (
			packageGUID = "the-package-guid"
			appGUID     = "the-app-guid"
			spaceGUID   = "the-space-guid"
			createdAt   = "1906-04-18T13:12:00Z"
			updatedAt   = "1906-04-18T13:12:01Z"
		)

		makeGetRequest := func(guid string) {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+guid, nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		When("on the happy path", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{
					GUID:      packageGUID,
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					State:     "AWAITING_UPLOAD",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
				}, nil)
			})

			JustBeforeEach(func() {
				makeGetRequest(packageGUID)
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("provides the authorization.Info from the request context to the package repository", func() {
				Expect(packageRepo.GetPackageCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := packageRepo.GetPackageArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a JSON body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`
				{
				  "guid": "` + packageGUID + `",
				  "type": "bits",
				  "data": {},
				  "state": "AWAITING_UPLOAD",
				  "created_at": "` + createdAt + `",
				  "updated_at": "` + updatedAt + `",
				  "relationships": {
					"app": {
					  "data": {
						"guid": "` + appGUID + `"
					  }
					}
				  },
				  "links": {
					"self": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID) + `"
					},
					"upload": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID, "/upload") + `",
					  "method": "POST"
					},
					"download": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID, "/download") + `",
					  "method": "GET"
					},
					"app": {
					  "href": "` + defaultServerURI("/v3/apps/", appGUID) + `"
					}
				  },
				  "metadata": {
					"labels": { },
					"annotations": { }
				  }
				}
            `))
			})
		})

		When("on the sad path", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, repositories.NotFoundError{})
			})

			JustBeforeEach(func() {
				makeGetRequest("invalid-package-guid")
			})

			It("returns status 404", func() {
				Expect(rr.Code).To(Equal(http.StatusNotFound), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a JSON body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`
				{
					"errors": [
						{
							"detail": "Package not found",
							"title": "CF-ResourceNotFound",
							"code": 10010
						}
					]
				}
            `))
			})
		})

		When("the authorization.Info is not set in the context", func() {
			JustBeforeEach(func() {
				ctx = context.Background()
				makeGetRequest(packageGUID)
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/packages endpoint", func() {
		var req *http.Request

		const (
			package1GUID = "the-package-guid"
			appGUID      = "the-app-guid"
			spaceGUID    = "the-space-guid"
			createdAt1   = "1906-04-18T13:12:00Z"
			updatedAt1   = "1906-04-18T13:12:01Z"
			package2GUID = "the-package-guid-2"
			createdAt2   = "1906-06-18T13:12:00Z"
			updatedAt2   = "1906-06-18T13:12:01Z"
		)

		BeforeEach(func() {
			packageRepo.ListPackagesReturns([]repositories.PackageRecord{
				{
					GUID:      package1GUID,
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					State:     "AWAITING_UPLOAD",
					CreatedAt: createdAt1,
					UpdatedAt: updatedAt1,
				},
				{
					GUID:      package2GUID,
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					State:     "READY",
					CreatedAt: createdAt2,
					UpdatedAt: updatedAt2,
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("on the happy path and", func() {
			When("multiple packages exist", func() {
				When("no query parameters are provided", func() {
					It("returns status 200", func() {
						Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
					})

					It("returns Content-Type as JSON in header", func() {
						contentTypeHeader := rr.Header().Get("Content-Type")
						Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
					})

					It("returns a JSON body", func() {
						Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(
							` {
							"pagination": {
								"total_results":2,
								"total_pages": 1,
								"first": {
									"href": "%[1]s/v3/packages"
								},
								"last": {
									"href": "%[1]s/v3/packages"
								},
								"next": null,
								"previous": null
							},
							"resources": [
								{
									"guid": "%[2]s",
									"type": "bits",
									"data": {},
									"state": "AWAITING_UPLOAD",
									"created_at": "%[3]s",
									"updated_at": "%[4]s",
									"relationships": {
										"app": {
											"data": {
												"guid": "%[5]s"
											}
										}
									},
									"links": {
										"self": {
											"href": "%[1]s/v3/packages/%[2]s"
										},
										"upload": {
											"href": "%[1]s/v3/packages/%[2]s/upload",
											"method": "POST"
										},
										"download": {
											"href": "%[1]s/v3/packages/%[2]s/download",
											"method": "GET"
										},
										"app": {
											"href": "%[1]s/v3/apps/%[5]s"
										}
									},
									"metadata": {
										"labels": {},
										"annotations": {}
									}
								},
								{
									"guid": "%[6]s",
									"type": "bits",
									"data": {},
									"state": "READY",
									"created_at": "%[7]s",
									"updated_at": "%[8]s",
									"relationships": {
										"app": {
											"data": {
												"guid": "%[5]s"
											}
										}
									},
									"links": {
										"self": {
											"href": "%[1]s/v3/packages/%[6]s"
										},
										"upload": {
											"href": "%[1]s/v3/packages/%[6]s/upload",
											"method": "POST"
										},
										"download": {
											"href": "%[1]s/v3/packages/%[6]s/download",
											"method": "GET"
										},
										"app": {
											"href": "%[1]s/v3/apps/%[5]s"
										}
									},
									"metadata": {
										"labels": {},
										"annotations": {}
									}
								}
							]
						}`, defaultServerURL, package1GUID, createdAt1, updatedAt1, appGUID, package2GUID, createdAt2, updatedAt2,
						)))
					})
				})

				When("the app_guids query parameter is provided", func() {
					BeforeEach(func() {
						var err error
						req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages?app_guids="+appGUID, nil)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns status 200", func() {
						Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
					})

					It("calls the package repository with expected arguments", func() {
						_, _, message := packageRepo.ListPackagesArgsForCall(0)
						Expect(message).To(Equal(repositories.ListPackagesMessage{AppGUIDs: []string{appGUID}}))
					})
				})

				When("the \"order_by\" parameter is sent", func() {
					BeforeEach(func() {
						var err error
						req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages?order_by=some_weird_value", nil)
						Expect(err).NotTo(HaveOccurred())
					})

					It("ignores it and returns status 200", func() {
						Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
					})
				})

				When("the \"per_page\" parameter is sent", func() {
					BeforeEach(func() {
						var err error
						req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages?per_page=some_weird_value", nil)
						Expect(err).NotTo(HaveOccurred())
					})

					It("ignores it and returns status 200", func() {
						Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
					})
				})
			})
			When("no packages exist", func() {
				BeforeEach(func() {
					packageRepo.ListPackagesReturns([]repositories.PackageRecord{}, nil)
				})

				It("returns status 200", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns a JSON body", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(
						` {
							"pagination": {
								"total_results": 0,
								"total_pages": 1,
								"first": {
									"href": "%[1]s/v3/packages"
								},
								"last": {
									"href": "%[1]s/v3/packages"
								},
								"next": null,
								"previous": null
							},
							"resources": []
						}`, defaultServerURL,
					)))
				})
			})
		})

		When("there is an unknown issue with the Package Repo", func() {
			BeforeEach(func() {
				packageRepo.ListPackagesReturns([]repositories.PackageRecord{}, errors.New("some-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("unsupported query parameters are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages?foo=my-app-guid", nil)
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'app_guids, order_by, per_page'")
			})
		})
	})

	Describe("the POST /v3/packages endpoint", func() {
		makePostRequest := func(body string) {
			req, err := http.NewRequestWithContext(ctx, "POST", "/v3/packages", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		const (
			packageGUID = "the-package-guid"
			appGUID     = "the-app-guid"
			appUID      = "the-app-uid"
			spaceGUID   = "the-space-guid"
			validBody   = `{
				"type": "bits",
				"relationships": {
					"app": {
						"data": {
							"guid": "` + appGUID + `"
						}
					}
				}
			}`
			createdAt = "1906-04-18T13:12:00Z"
			updatedAt = "1906-04-18T13:12:01Z"
		)

		BeforeEach(func() {
			packageRepo.CreatePackageReturns(repositories.PackageRecord{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				GUID:      packageGUID,
				State:     "AWAITING_UPLOAD",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil)

			appRepo.GetAppReturns(repositories.AppRecord{
				SpaceGUID: spaceGUID,
				GUID:      appGUID,
				EtcdUID:   appUID,
			}, nil)
		})

		When("on the happy path", func() {
			JustBeforeEach(func() {
				makePostRequest(validBody)
			})

			It("returns status 201", func() {
				Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("creates a CFPackage", func() {
				Expect(packageRepo.CreatePackageCallCount()).To(Equal(1))
				_, actualAuthInfo, actualCreate := packageRepo.CreatePackageArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualCreate).To(Equal(repositories.CreatePackageMessage{
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					OwnerRef: metav1.OwnerReference{
						APIVersion: "workloads.cloudfoundry.org/v1alpha1",
						Kind:       "CFApp",
						Name:       appGUID,
						UID:        appUID,
					},
				}))
			})

			It("returns a JSON body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`
				{
				  "guid": "` + packageGUID + `",
				  "type": "bits",
				  "data": {},
				  "state": "AWAITING_UPLOAD",
				  "created_at": "` + createdAt + `",
				  "updated_at": "` + updatedAt + `",
				  "relationships": {
					"app": {
					  "data": {
						"guid": "` + appGUID + `"
					  }
					}
				  },
				  "links": {
					"self": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID) + `"
					},
					"upload": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID, "/upload") + `",
					  "method": "POST"
					},
					"download": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID, "/download") + `",
					  "method": "GET"
					},
					"app": {
					  "href": "` + defaultServerURI("/v3/apps/", appGUID) + `"
					}
				  },
				  "metadata": {
					"labels": { },
					"annotations": { }
				  }
				}
            `))
			})
		})

		itDoesntCreateAPackage := func() {
			It("doesn't create a package", func() {
				Expect(packageRepo.CreatePackageCallCount()).To(Equal(0))
			})
		}

		When("the app doesn't exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			JustBeforeEach(func() {
				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("App is invalid. Ensure it exists and you have access to it.")
			})
			itDoesntCreateAPackage()
		})

		When("the app exists check returns an error", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			JustBeforeEach(func() {
				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntCreateAPackage()
		})

		When("the type is invalid", func() {
			const (
				bodyWithInvalidType = `{
					"type": "docker",
					"relationships": {
						"app": {
							"data": {
								"guid": "` + appGUID + `"
							}
						}
					}
				}`
			)

			JustBeforeEach(func() {
				makePostRequest(bodyWithInvalidType)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Type must be one of ['bits']")
			})
		})

		When("the relationship field is completely omitted", func() {
			JustBeforeEach(func() {
				makePostRequest(`{ "type": "bits" }`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Relationships is a required field")
			})
		})

		When("an invalid relationship is given", func() {
			const bodyWithoutAppRelationship = `{
				"type": "bits",
				"relationships": {
					"build": {
						"data": {}
				   	}
				}
			}`

			JustBeforeEach(func() {
				makePostRequest(bodyWithoutAppRelationship)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "build"`)
			})
		})

		When("the JSON body is invalid", func() {
			JustBeforeEach(func() {
				makePostRequest(`{`)
			})

			It("returns a status 400 Bad Request ", func() {
				Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
					"errors": [
						{
							"title": "CF-MessageParseError",
							"detail": "Request invalid due to parse error: invalid request body",
							"code": 1001
						}
					]
				}`), "Response body matches response:")
			})
		})

		When("creating the package in the repo errors", func() {
			BeforeEach(func() {
				packageRepo.CreatePackageReturns(repositories.PackageRecord{}, errors.New("boom"))
			})

			JustBeforeEach(func() {
				makePostRequest(validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the authorization.Info is not set in the context", func() {
			BeforeEach(func() {
				ctx = context.Background()
			})

			JustBeforeEach(func() {
				makePostRequest(validBody)
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/packages/upload endpoint", func() {
		var credentialOption remote.Option

		makeUploadRequest := func(packageGUID string, file io.Reader) {
			var b bytes.Buffer
			writer := multipart.NewWriter(&b)
			part, err := writer.CreateFormFile("bits", "unused.zip")
			Expect(err).NotTo(HaveOccurred())

			_, err = io.Copy(part, file)
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.Close()).To(Succeed())

			req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("/v3/packages/%s/upload", packageGUID), &b)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-Type", writer.FormDataContentType())

			router.ServeHTTP(rr, req)
		}

		const (
			packageGUID        = "the-package-guid"
			appGUID            = "the-app-guid"
			createdAt          = "1906-04-18T13:12:00Z"
			updatedAt          = "1906-04-18T13:12:01Z"
			imageRefWithDigest = "some-org/the-package-guid@SHA256:some-sha-256"
			srcFileContents    = "the-src-file-contents"
		)

		BeforeEach(func() {
			packageRepo.GetPackageReturns(repositories.PackageRecord{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				GUID:      packageGUID,
				State:     "AWAITING_UPLOAD",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil)

			packageRepo.UpdatePackageSourceReturns(repositories.PackageRecord{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				GUID:      packageGUID,
				State:     "READY",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil)

			packageRegistryBase = "some-org"
			packageImagePullSecretName = "package-image-pull-secret"

			uploadImageSource.Returns(imageRefWithDigest, nil)

			credentialOption = remote.WithUserAgent("for-test-use-only") // real one should have credentials
			buildRegistryAuth.Returns(credentialOption, nil)
		})

		When("on the happy path", func() {
			JustBeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader(srcFileContents))
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("fetches the right package", func() {
				Expect(packageRepo.GetPackageCallCount()).To(Equal(1))

				_, actualAuthInfo, actualPackageGUID := packageRepo.GetPackageArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualPackageGUID).To(Equal(packageGUID))
			})

			It("uploads the image source", func() {
				Expect(uploadImageSource.CallCount()).To(Equal(1))
				imageRef, srcFile, actualCredentialOption := uploadImageSource.ArgsForCall(0)
				Expect(imageRef).To(Equal(fmt.Sprintf("%s/%s", packageRegistryBase, packageGUID)))
				actualSrcContents, err := io.ReadAll(srcFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actualSrcContents)).To(Equal(srcFileContents))
				Expect(actualCredentialOption).NotTo(BeNil())
			})

			It("saves the uploaded image reference on the package", func() {
				Expect(packageRepo.UpdatePackageSourceCallCount()).To(Equal(1))
				_, actualAuthInfo, message := packageRepo.UpdatePackageSourceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(message.GUID).To(Equal(packageGUID))
				Expect(message.ImageRef).To(Equal(imageRefWithDigest))
				Expect(message.RegistrySecretName).To(Equal(packageImagePullSecretName))
			})

			It("returns a JSON body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`
				{
				  "guid": "` + packageGUID + `",
				  "type": "bits",
				  "data": {},
				  "state": "READY",
				  "created_at": "` + createdAt + `",
				  "updated_at": "` + updatedAt + `",
				  "relationships": {
					"app": {
					  "data": {
						"guid": "` + appGUID + `"
					  }
					}
				  },
				  "links": {
					"self": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID) + `"
					},
					"upload": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID, "/upload") + `",
					  "method": "POST"
					},
					"download": {
					  "href": "` + defaultServerURI("/v3/packages/", packageGUID, "/download") + `",
					  "method": "GET"
					},
					"app": {
					  "href": "` + defaultServerURI("/v3/apps/", appGUID) + `"
					}
				  },
				  "metadata": {
					"labels": { },
					"annotations": { }
				  }
				}
            `))
			})
		})

		itDoesntBuildAnImageFromSource := func() {
			It("doesn't build an image from the source", func() {
				Expect(uploadImageSource.CallCount()).To(Equal(0))
			})
		}

		itDoesntUpdateAnyPackages := func() {
			It("doesn't update any Packages", func() {
				Expect(packageRepo.UpdatePackageSourceCallCount()).To(Equal(0))
			})
		}

		When("the record doesn't exist", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, repositories.NotFoundError{})
			})

			JustBeforeEach(func() {
				makeUploadRequest("no-such-package-guid", strings.NewReader("the-zip-contents"))
			})

			It("returns an error", func() {
				expectNotFoundError("Package not found")
			})
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("the authorization.Info is not set in the request context", func() {
			JustBeforeEach(func() {
				ctx = context.Background()
				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("fetching the package errors", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, errors.New("boom"))
			})

			JustBeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("no bits file is given", func() {
			JustBeforeEach(func() {
				var b bytes.Buffer
				writer := multipart.NewWriter(&b)
				Expect(writer.Close()).To(Succeed())

				req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("/v3/packages/%s/upload", packageGUID), &b)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-Type", writer.FormDataContentType())

				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Upload must include bits")
			})
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("building the image credentials errors", func() {
			BeforeEach(func() {
				buildRegistryAuth.Returns(nil, errors.New("boom"))
			})

			JustBeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("uploading the source image errors", func() {
			BeforeEach(func() {
				uploadImageSource.Returns("", errors.New("boom"))
			})

			JustBeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntUpdateAnyPackages()
		})

		When("updating the package source registry errors", func() {
			BeforeEach(func() {
				packageRepo.UpdatePackageSourceReturns(repositories.PackageRecord{}, errors.New("boom"))
			})

			JustBeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the package has already been uploaded", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					GUID:      packageGUID,
					State:     repositories.PackageStateReady,
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
				}, nil)
			})

			JustBeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			It("returns status 400 BadRequest", func() {
				Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
			})

			It("returns a CF API formatted Error response", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
					"errors": [
						{
							"title": "CF-PackageBitsAlreadyUploaded",
							"detail": "Bits may be uploaded only once. Create a new package to upload different bits.",
							"code": 150004
						}
					]
				}`), "Response body matches response:")
			})
		})
	})

	Describe("the GET /v3/packages/:guid/droplets endpoint", func() {
		var req *http.Request

		const (
			packageGUID = "the-package-guid"
			dropletGUID = "the-droplet-guid"
			appGUID     = "the-app-guid"
			// spaceGUID    = "the-space-guid"
			createdAt = "1906-04-18T13:12:00Z"
			updatedAt = "1906-04-18T13:12:01Z"
			// package2GUID = "the-package-guid-2"
			// createdAt2   = "1906-06-18T13:12:00Z"
			// updatedAt2   = "1906-06-18T13:12:01Z"
		)

		BeforeEach(func() {
			packageRepo.GetPackageReturns(repositories.PackageRecord{
				GUID: packageGUID,
			}, nil)

			dropletRepo.ListDropletsReturns([]repositories.DropletRecord{
				{
					GUID:      dropletGUID,
					State:     "STAGED",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
					Stack: "cflinuxfs3",
					ProcessTypes: map[string]string{
						"web": "bundle exec rackup config.ru -p $PORT",
					},
					AppGUID:     appGUID,
					PackageGUID: packageGUID,
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID+"/droplets", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("on the happy path and", func() {
			When("multiple droplets exist", func() {
				It("returns status 200", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("fetches the right package", func() {
					Expect(packageRepo.GetPackageCallCount()).To(Equal(1))

					_, _, actualPackageGUID := packageRepo.GetPackageArgsForCall(0)
					Expect(actualPackageGUID).To(Equal(packageGUID))
				})

				It("retrieves the droplets for the specified package", func() {
					Expect(dropletRepo.ListDropletsCallCount()).To(Equal(1))

					_, _, dropletListMessage := dropletRepo.ListDropletsArgsForCall(0)
					Expect(dropletListMessage).To(Equal(repositories.ListDropletsMessage{
						PackageGUIDs: []string{packageGUID},
					}))
				})

				It("returns the droplet in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
						"pagination": {
							"total_results": 1,
							"total_pages": 1,
							"first": {
								"href": "%[1]s/v3/packages/%[2]s/droplets"
							},
							"last": {
								"href": "%[1]s/v3/packages/%[2]s/droplets"
							},
							"next": null,
							"previous": null
						},
						"resources": [
							{
								"guid": "%[4]s",
								"state": "STAGED",
								"error": null,
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [],
										"stack": ""
									}
								},
								"execution_metadata": "",
								"process_types": {
									"web": "bundle exec rackup config.ru -p $PORT"
								},
								"checksum": null,
								"buildpacks": [],
								"stack": "cflinuxfs3",
								"image": null,
								"created_at": "%[5]s",
								"updated_at": "%[6]s",
								"relationships": {
									"app": {
										"data": {
											"guid": "%[3]s"
										}
									}
								},
								"links": {
									"self": {
										"href": "%[1]s/v3/droplets/%[4]s"
									},
									"package": {
										"href": "%[1]s/v3/packages/%[2]s"
									},
									"app": {
										"href": "%[1]s/v3/apps/%[3]s"
									},
									"assign_current_droplet": {
										"href": "%[1]s/v3/apps/%[3]s/relationships/current_droplet",
										"method": "PATCH"
									},
									"download": null
								},
								"metadata": {
									"labels": {},
									"annotations": {}
								}
							}
						]
					}`, defaultServerURL, packageGUID, appGUID, dropletGUID, createdAt, updatedAt)), "Response body matches response:")
				})
			})

			When("the \"states\" query parameter is provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID+"/droplets?states=SOME_WEIRD_VALUE", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("ignores it and returns status 200", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})
			})

			When("the \"per_page\" query parameter is provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID+"/droplets?per_page=SOME_WEIRD_VALUE", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("ignores it and returns status 200", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})
			})
		})

		When("on the sad path and", func() {
			When("the package does not exist", func() {
				BeforeEach(func() {
					packageRepo.GetPackageReturns(repositories.PackageRecord{}, repositories.NotFoundError{})
				})

				It("returns the error", func() {
					expectNotFoundError("Package not found")
				})
			})

			When("an error occurs while fetching the package", func() {
				BeforeEach(func() {
					packageRepo.GetPackageReturns(repositories.PackageRecord{}, errors.New("boom"))
				})

				It("returns the error", func() {
					expectUnknownError()
				})
			})

			When("an error occurs while fetching the droplets for the package", func() {
				BeforeEach(func() {
					dropletRepo.ListDropletsReturns([]repositories.DropletRecord{}, errors.New("boom"))
				})

				It("returns the error", func() {
					expectUnknownError()
				})
			})

			When("unsupported query parameters are provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID+"/droplets?foo=my-app-guid", nil)
					Expect(err).NotTo(HaveOccurred())
				})
				It("returns an Unknown key error", func() {
					expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'states, per_page'")
				})
			})
		})
	})
})
