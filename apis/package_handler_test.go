package apis_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	testPackageHandlerLoggerName = "TestPackageHandler"
)

var _ = Describe("PackageHandler", func() {
	Describe("the POST /v3/packages endpoint", func() {
		var (
			rr            *httptest.ResponseRecorder
			packageRepo   *fake.CFPackageRepository
			appRepo       *fake.CFAppRepository
			clientBuilder *fake.ClientBuilder
			router        *mux.Router
		)

		makePostRequest := func(body string) {
			req, err := http.NewRequest("POST", "/v3/packages", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		const (
			packageGUID = "the-package-guid"
			appGUID     = "the-app-guid"
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

		getRR := func() *httptest.ResponseRecorder { return rr }

		BeforeEach(func() {
			rr = httptest.NewRecorder()
			router = mux.NewRouter()

			packageRepo = new(fake.CFPackageRepository)
			packageRepo.CreatePackageReturns(repositories.PackageRecord{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				GUID:      packageGUID,
				State:     "AWAITING_UPLOAD",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil)

			appRepo = new(fake.CFAppRepository)
			appRepo.FetchAppReturns(repositories.AppRecord{
				SpaceGUID: spaceGUID,
			}, nil)

			clientBuilder = new(fake.ClientBuilder)

			apiHandler := NewPackageHandler(logf.Log.WithName(testPackageHandlerLoggerName), defaultServerURL, packageRepo, appRepo, clientBuilder.Spy, nil, nil, &rest.Config{}, "", "")
			apiHandler.RegisterRoutes(router)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				makePostRequest(validBody)
			})

			It("returns status 201", func() {
				Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("configures the client", func() {
				Expect(clientBuilder.CallCount()).To(Equal(1))
			})

			It("creates a CFPackage", func() {
				Expect(packageRepo.CreatePackageCallCount()).To(Equal(1))
				_, _, actualCreate := packageRepo.CreatePackageArgsForCall(0)
				Expect(actualCreate).To(Equal(repositories.PackageCreateMessage{
					Type:      "bits",
					AppGUID:   appGUID,
					SpaceGUID: spaceGUID,
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
				appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})

				makePostRequest(validBody)
			})

			itRespondsWithUnprocessableEntity("App is invalid. Ensure it exists and you have access to it.", getRR)
			itDoesntCreateAPackage()
		})

		When("the app exists check returns an error", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("boom"))

				makePostRequest(validBody)
			})

			itRespondsWithUnknownError(getRR)
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

			BeforeEach(func() {
				makePostRequest(bodyWithInvalidType)
			})

			itRespondsWithUnprocessableEntity("Type must be one of ['bits']", getRR)
		})

		When("the relationship field is completely omitted", func() {
			BeforeEach(func() {
				makePostRequest(`{ "type": "bits" }`)
			})

			itRespondsWithUnprocessableEntity("Relationships is a required field", getRR)
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

			BeforeEach(func() {
				makePostRequest(bodyWithoutAppRelationship)
			})

			itRespondsWithUnprocessableEntity(`invalid request body: json: unknown field "build"`, getRR)
		})

		When("the JSON body is invalid", func() {
			BeforeEach(func() {
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

		When("building the k8s client errors", func() {
			BeforeEach(func() {
				clientBuilder.Returns(nil, errors.New("boom"))
				makePostRequest(validBody)
			})

			itRespondsWithUnknownError(getRR)
			itDoesntCreateAPackage()
		})

		When("creating the package in the repo errors", func() {
			BeforeEach(func() {
				packageRepo.CreatePackageReturns(repositories.PackageRecord{}, errors.New("boom"))
				makePostRequest(validBody)
			})

			itRespondsWithUnknownError(getRR)
		})
	})

	Describe("the POST /v3/packages/upload endpoint", func() {
		var (
			rr                *httptest.ResponseRecorder
			packageRepo       *fake.CFPackageRepository
			appRepo           *fake.CFAppRepository
			uploadImageSource *fake.SourceImageUploader
			buildRegistryAuth *fake.RegistryAuthBuilder
			credentialOption  remote.Option
			clientBuilder     *fake.ClientBuilder
			router            *mux.Router
		)

		getRR := func() *httptest.ResponseRecorder { return rr }

		makeUploadRequest := func(packageGUID string, file io.Reader) {
			var b bytes.Buffer
			writer := multipart.NewWriter(&b)
			part, err := writer.CreateFormFile("bits", "unused.zip")
			Expect(err).NotTo(HaveOccurred())

			_, err = io.Copy(part, file)
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.Close()).To(Succeed())

			req, err := http.NewRequest("POST", fmt.Sprintf("/v3/packages/%s/upload", packageGUID), &b)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-Type", writer.FormDataContentType())

			router.ServeHTTP(rr, req)
		}

		const (
			packageGUID                = "the-package-guid"
			appGUID                    = "the-app-guid"
			createdAt                  = "1906-04-18T13:12:00Z"
			updatedAt                  = "1906-04-18T13:12:01Z"
			imageRefWithDigest         = "some-org/the-package-guid@SHA256:some-sha-256"
			srcFileContents            = "the-src-file-contents"
			packageRegistryBase        = "some-org"
			packageImagePullSecretName = "package-image-pull-secret"
		)

		BeforeEach(func() {
			rr = httptest.NewRecorder()
			router = mux.NewRouter()

			packageRepo = new(fake.CFPackageRepository)
			packageRepo.FetchPackageReturns(repositories.PackageRecord{
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
				State:     "PROCESSING_UPLOAD",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil)

			uploadImageSource = new(fake.SourceImageUploader)
			uploadImageSource.Returns(imageRefWithDigest, nil)

			appRepo = new(fake.CFAppRepository)
			clientBuilder = new(fake.ClientBuilder)
			credentialOption = remote.WithUserAgent("for-test-use-only") // real one should have credentials
			buildRegistryAuth = new(fake.RegistryAuthBuilder)
			buildRegistryAuth.Returns(credentialOption, nil)

			apiHandler := NewPackageHandler(
				logf.Log.WithName(testPackageHandlerLoggerName),
				defaultServerURL,
				packageRepo,
				appRepo,
				clientBuilder.Spy,
				uploadImageSource.Spy,
				buildRegistryAuth.Spy,
				&rest.Config{},
				packageRegistryBase,
				packageImagePullSecretName,
			)

			apiHandler.RegisterRoutes(router)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				makeUploadRequest(packageGUID, strings.NewReader(srcFileContents))
			})

			It("returns status 200", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("configures the client", func() {
				Expect(clientBuilder.CallCount()).To(Equal(1))
			})

			It("fetches the right package", func() {
				Expect(packageRepo.FetchPackageCallCount()).To(Equal(1))

				_, _, actualPackageGUID := packageRepo.FetchPackageArgsForCall(0)
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
				_, _, message := packageRepo.UpdatePackageSourceArgsForCall(0)
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
				  "state": "PROCESSING_UPLOAD",
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
				packageRepo.FetchPackageReturns(repositories.PackageRecord{}, repositories.NotFoundError{})

				makeUploadRequest("no-such-package-guid", strings.NewReader("the-zip-contents"))
			})

			itRespondsWithNotFound("Package not found", getRR)
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("building the client errors", func() {
			BeforeEach(func() {
				clientBuilder.Returns(nil, errors.New("boom"))

				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("fetching the package errors", func() {
			BeforeEach(func() {
				packageRepo.FetchPackageReturns(repositories.PackageRecord{}, errors.New("boom"))

				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("no bits file is given", func() {
			BeforeEach(func() {
				var b bytes.Buffer
				writer := multipart.NewWriter(&b)
				Expect(writer.Close()).To(Succeed())

				req, err := http.NewRequest("POST", fmt.Sprintf("/v3/packages/%s/upload", packageGUID), &b)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-Type", writer.FormDataContentType())

				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnprocessableEntity("Upload must include bits", getRR)
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("building the image credentials errors", func() {
			BeforeEach(func() {
				buildRegistryAuth.Returns(nil, errors.New("boom"))

				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntBuildAnImageFromSource()
			itDoesntUpdateAnyPackages()
		})

		When("uploading the source image errors", func() {
			BeforeEach(func() {
				uploadImageSource.Returns("", errors.New("boom"))

				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntUpdateAnyPackages()
		})

		When("updating the package source registry errors", func() {
			BeforeEach(func() {
				packageRepo.UpdatePackageSourceReturns(repositories.PackageRecord{}, errors.New("boom"))

				makeUploadRequest(packageGUID, strings.NewReader("the-zip-contents"))
			})

			itRespondsWithUnknownError(getRR)
		})
	})
})
