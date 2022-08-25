package handlers_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PackageHandler", func() {
	var (
		packageRepo                *fake.CFPackageRepository
		appRepo                    *fake.CFAppRepository
		dropletRepo                *fake.CFDropletRepository
		imageRepo                  *fake.ImageRepository
		packageRegistryBase        string
		packageImagePullSecretName string

		packageGUID string
		appGUID     string
		spaceGUID   string
		createdAt   string
		updatedAt   string
	)

	BeforeEach(func() {
		packageRepo = new(fake.CFPackageRepository)
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		imageRepo = new(fake.ImageRepository)
		packageRegistryBase = "some-org"
		packageImagePullSecretName = "package-image-pull-secret"

		packageGUID = generateGUID("package")
		appGUID = generateGUID("app")
		spaceGUID = generateGUID("space")
		createdAt = time.Now().Format(time.RFC3339)
		updatedAt = time.Now().Format(time.RFC3339)

		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewPackageHandler(
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			imageRepo,
			decoderValidator,
			packageRegistryBase,
			packageImagePullSecretName,
		)

		apiHandler.RegisterRoutes(router)
	})

	Describe("the GET /v3/packages/:guid endpoint", func() {
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
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
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

		When("getting the package returns a forbidden error", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, apierrors.NewForbiddenError(nil, repositories.PackageResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Package not found")
			})
		})

		When("getting the package fails", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/packages endpoint", func() {
		var (
			req              *http.Request
			queryParamString string

			anotherPackageGUID string
		)

		BeforeEach(func() {
			queryParamString = ""

			anotherPackageGUID = generateGUID("package2")

			packageRepo.ListPackagesReturns([]repositories.PackageRecord{
				{
					GUID:      packageGUID,
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					State:     "AWAITING_UPLOAD",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
				},
				{
					GUID:      anotherPackageGUID,
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
					State:     "READY",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
				},
			}, nil)
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/packages"+queryParamString, nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
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
						}`, defaultServerURL, packageGUID, createdAt, updatedAt, appGUID, anotherPackageGUID, createdAt, updatedAt,
			)))
		})

		When("the 'app_guids' query parameter is provided", func() {
			BeforeEach(func() {
				queryParamString = "?app_guids=" + appGUID
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("calls the package repository with expected arguments", func() {
				_, _, message := packageRepo.ListPackagesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListPackagesMessage{
					AppGUIDs: []string{appGUID},
					States:   []string{},
				}))
			})
		})

		When("an invalid 'order_by' parameter is sent", func() {
			BeforeEach(func() {
				queryParamString = "?order_by=some_weird_value"
			})

			It("ignores it and returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("an 'order_by' parameter is sent", func() {
			BeforeEach(func() {
				queryParamString = "?order_by=created_at"
			})

			It("calls repository ListPackage with the correct message object", func() {
				_, _, message := packageRepo.ListPackagesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListPackagesMessage{
					AppGUIDs:        []string{},
					SortBy:          "created_at",
					DescendingOrder: false,
					States:          []string{},
				}))
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("an 'order_by' descending parameter is sent", func() {
			BeforeEach(func() {
				queryParamString = "?order_by=-created_at"
			})

			It("calls repository ListPackage with the correct message object", func() {
				_, _, message := packageRepo.ListPackagesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListPackagesMessage{
					AppGUIDs:        []string{},
					SortBy:          "created_at",
					DescendingOrder: true,
					States:          []string{},
				}))
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("the 'per_page' parameter is sent", func() {
			BeforeEach(func() {
				queryParamString = "?per_page=some_weird_value"
			})

			It("ignores it and returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("the 'states' parameter is sent", func() {
			BeforeEach(func() {
				queryParamString = "?states=READY,AWAITING_UPLOAD"
			})

			It("calls repository ListPackage with the correct message object", func() {
				_, _, message := packageRepo.ListPackagesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListPackagesMessage{
					AppGUIDs:        []string{},
					SortBy:          "",
					DescendingOrder: false,
					States:          []string{"READY", "AWAITING_UPLOAD"},
				}))
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("unsupported query parameters are provided", func() {
			BeforeEach(func() {
				queryParamString = "?foo=my-app-guid"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'app_guids, order_by, per_page, states'")
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

		When("there is an unknown issue with the Package Repo", func() {
			BeforeEach(func() {
				packageRepo.ListPackagesReturns([]repositories.PackageRecord{}, errors.New("some-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/packages endpoint", func() {
		var (
			appUID types.UID
			body   string
		)

		BeforeEach(func() {
			appUID = "appUID"
			body = `{
				"type": "bits",
				"relationships": {
					"app": {
						"data": {
							"guid": "` + appGUID + `"
						}
					}
				}
			}`

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

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", "/v3/packages", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
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
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
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

		itDoesntCreateAPackage := func() {
			It("doesn't create a package", func() {
				Expect(packageRepo.CreatePackageCallCount()).To(Equal(0))
			})
		}

		When("the app doesn't exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(errors.New("NotFound"), repositories.AppResourceType))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("App is invalid. Ensure it exists and you have access to it.")
			})

			itDoesntCreateAPackage()
		})

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(errors.New("Forbidden"), repositories.AppResourceType))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("App is invalid. Ensure it exists and you have access to it.")
			})

			itDoesntCreateAPackage()
		})

		When("the app exists check returns an error", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			itDoesntCreateAPackage()
		})

		When("the type is invalid", func() {
			BeforeEach(func() {
				body = `{
					"type": "docker",
					"relationships": {
						"app": {
							"data": {
								"guid": "` + appGUID + `"
							}
						}
					}
				}`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Type must be one of ['bits']")
			})
		})

		When("the relationship field is completely omitted", func() {
			BeforeEach(func() {
				body = `{ "type": "bits" }`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Relationships is a required field")
			})
		})

		When("an invalid relationship is given", func() {
			BeforeEach(func() {
				body = `{
                    "type": "bits",
                    "relationships": {
                        "build": {
                            "data": {}
                        }
                    }
                }`
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "build"`)
			})
		})

		When("the JSON body is invalid", func() {
			BeforeEach(func() {
				body = "{"
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

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/packages/upload endpoint", func() {
		var (
			imageRefWithDigest string
			body               io.Reader
			formDataHeader     string
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

			imageRefWithDigest = "some-org/the-package-guid@SHA256:some-sha-256"
			imageRepo.UploadSourceImageReturns(imageRefWithDigest, nil)

			var b bytes.Buffer
			writer := multipart.NewWriter(&b)
			part, err := writer.CreateFormFile("bits", "unused.zip")
			Expect(err).NotTo(HaveOccurred())
			_, err = io.Copy(part, strings.NewReader("the-src-file-contents"))
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.Close()).To(Succeed())
			formDataHeader = writer.FormDataContentType()

			body = &b
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("/v3/packages/%s/upload", packageGUID), body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-Type", formDataHeader)

			router.ServeHTTP(rr, req)
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
			Expect(imageRepo.UploadSourceImageCallCount()).To(Equal(1))
			_, actualAuthInfo, imageRef, srcFile, actualSpaceGUID := imageRepo.UploadSourceImageArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(imageRef).To(Equal(fmt.Sprintf("%s/%s", packageRegistryBase, packageGUID)))
			actualSrcContents, err := io.ReadAll(srcFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actualSrcContents)).To(Equal("the-src-file-contents"))
			Expect(actualSpaceGUID).To(Equal(spaceGUID))
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

		itDoesntUploadSourceImage := func() {
			It("doesn't build an image from the source", func() {
				Expect(imageRepo.UploadSourceImageCallCount()).To(Equal(0))
			})
		}

		itDoesntUpdateAnyPackages := func() {
			It("doesn't update any Packages", func() {
				Expect(packageRepo.UpdatePackageSourceCallCount()).To(Equal(0))
			})
		}

		When("getting the package is forbidden", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, apierrors.NewForbiddenError(errors.New("Forbidden"), repositories.PackageResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Package not found")
			})
			itDoesntUploadSourceImage()
			itDoesntUpdateAnyPackages()
		})

		When("the getting the package errors", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntUploadSourceImage()
			itDoesntUpdateAnyPackages()
		})

		When("uploading the package is forbidden", func() {
			BeforeEach(func() {
				imageRepo.UploadSourceImageReturns("", apierrors.NewForbiddenError(errors.New("Forbidden"), repositories.PackageResourceType))
			})

			It("returns an error", func() {
				expectNotAuthorizedError()
			})
		})

		When("no bits file is given", func() {
			BeforeEach(func() {
				var b bytes.Buffer
				writer := multipart.NewWriter(&b)
				Expect(writer.Close()).To(Succeed())
				body = &b
				formDataHeader = writer.FormDataContentType()
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Upload must include bits")
			})
			itDoesntUploadSourceImage()
			itDoesntUpdateAnyPackages()
		})

		When("preparing to upload the source image errors", func() {
			BeforeEach(func() {
				imageRepo.UploadSourceImageReturns("", errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntUpdateAnyPackages()
		})

		When("uploading the source image errors", func() {
			BeforeEach(func() {
				imageRepo.UploadSourceImageReturns("", apierrors.NewBlobstoreUnavailableError(errors.New("boom")))
			})

			It("returns an error", func() {
				expectBlobstoreUnavailableError()
			})
			itDoesntUpdateAnyPackages()
		})

		When("updating the package source registry errors", func() {
			BeforeEach(func() {
				packageRepo.UpdatePackageSourceReturns(repositories.PackageRecord{}, errors.New("boom"))
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
		var dropletGUID string
		var queryString string

		BeforeEach(func() {
			packageRepo.GetPackageReturns(repositories.PackageRecord{
				GUID: packageGUID,
			}, nil)

			dropletGUID = generateGUID("droplet")
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
			queryString = ""
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID+"/droplets"+queryString, nil)
			Expect(err).NotTo(HaveOccurred())
			router.ServeHTTP(rr, req)
		})

		It("returns status 200", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", jsonHeader))
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

		When("the 'states' query parameter is provided", func() {
			BeforeEach(func() {
				queryString = "?states=SOME_WEIRD_VALUE"
			})

			It("ignores it and returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("the \"per_page\" query parameter is provided", func() {
			BeforeEach(func() {
				queryString = "?per_page=SOME_WEIRD_VALUE"
			})

			It("ignores it and returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
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

		When("a forbidden error occurs while fetching the package", func() {
			BeforeEach(func() {
				toReturnErr := apierrors.NewForbiddenError(nil, repositories.PackageResourceType)
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, toReturnErr)
			})

			It("returns the error", func() {
				expectNotFoundError("Package")
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
				queryString = "?foo=my-app-guid"
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'states, per_page'")
			})
		})
	})
})
