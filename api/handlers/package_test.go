package handlers_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Package", func() {
	var (
		packageRepo                 *fake.CFPackageRepository
		appRepo                     *fake.CFAppRepository
		dropletRepo                 *fake.CFDropletRepository
		imageRepo                   *fake.ImageRepository
		requestValidator            *fake.RequestValidator
		packageImagePullSecretNames []string

		packageGUID string
		appGUID     string
		spaceGUID   string
		createdAt   time.Time
		updatedAt   *time.Time
	)

	BeforeEach(func() {
		packageRepo = new(fake.CFPackageRepository)
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		imageRepo = new(fake.ImageRepository)
		requestValidator = new(fake.RequestValidator)
		packageImagePullSecretNames = []string{"package-image-pull-secret"}

		packageGUID = generateGUID("package")
		appGUID = generateGUID("app")
		spaceGUID = generateGUID("space")
		createdAt = time.Now()
		updatedAt = tools.PtrTo(time.Now())

		apiHandler := NewPackage(
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			imageRepo,
			requestValidator,
			packageImagePullSecretNames,
		)

		routerBuilder.LoadRoutes(apiHandler)
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
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"baz": "fof",
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the package", func() {
			Expect(packageRepo.GetPackageCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := packageRepo.GetPackageArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", packageGUID),
				MatchJSONPath("$.state", "AWAITING_UPLOAD"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("getting the package returns a forbidden error", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, apierrors.NewForbiddenError(nil, repositories.PackageResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Package")
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
		var anotherPackageGUID string

		BeforeEach(func() {
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

			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.PackageList{})
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/packages?foo=bar", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the package list", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix("foo=bar"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/packages?foo=bar"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", packageGUID),
				MatchJSONPath("$.resources[0].state", Equal("AWAITING_UPLOAD")),
				MatchJSONPath("$.resources[1].guid", anotherPackageGUID),
			)))
		})

		When("the 'app_guids' query parameter is provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.PackageList{
					AppGUIDs: appGUID,
				})
			})

			It("calls the package repository with expected arguments", func() {
				_, _, message := packageRepo.ListPackagesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListPackagesMessage{
					AppGUIDs: []string{appGUID},
					States:   []string{},
				}))

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})
		})

		Describe("Order results", func() {
			BeforeEach(func() {
				packageRepo.ListPackagesReturns([]repositories.PackageRecord{
					{
						GUID:      "1",
						CreatedAt: time.UnixMilli(3000),
						UpdatedAt: tools.PtrTo(time.UnixMilli(4000)),
					},
					{
						GUID:      "2",
						CreatedAt: time.UnixMilli(2000),
						UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
					},
					{
						GUID:      "3",
						CreatedAt: time.UnixMilli(1000),
						UpdatedAt: tools.PtrTo(time.UnixMilli(5000)),
					},
				}, nil)
			})

			DescribeTable("ordering results", func(orderBy string, expectedOrder ...any) {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(
					&payloads.PackageList{
						OrderBy: orderBy,
					},
				)

				req := createHttpRequest("GET", "/v3/packages?order_by=not-used", nil)
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.resources[*].guid", expectedOrder)))
			},
				Entry("created_at ASC", "created_at", "3", "2", "1"),
				Entry("created_at DESC", "-created_at", "1", "2", "3"),
				Entry("updated_at ASC", "updated_at", "2", "1", "3"),
				Entry("updated_at DESC", "-updated_at", "3", "1", "2"),
			)
		})

		When("the 'states' parameter is sent", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(
					&payloads.PackageList{
						States: "READY,AWAITING_UPLOAD",
					},
				)
			})

			It("calls repository ListPackage with the correct message object", func() {
				_, _, message := packageRepo.ListPackagesArgsForCall(0)
				Expect(message).To(Equal(repositories.ListPackagesMessage{
					AppGUIDs: []string{},
					States:   []string{"READY", "AWAITING_UPLOAD"},
				}))
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})
		})

		When("request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("foo"))
			})

			It("returns an Unknown error", func() {
				expectUnknownError()
			})
		})

		When("no packages exist", func() {
			BeforeEach(func() {
				packageRepo.ListPackagesReturns([]repositories.PackageRecord{}, nil)
			})

			It("returns an empty list", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.pagination.total_results", BeEquivalentTo(0)),
					MatchJSONPath("$.resources", BeEmpty()),
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
		var appUID types.UID

		BeforeEach(func() {
			appUID = "appUID"

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.PackageCreate{
				Type: "bits",
				Relationships: &payloads.PackageRelationships{
					App: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: appGUID,
						},
					},
				},
				Metadata: payloads.Metadata{
					Labels: map[string]string{
						"bob": "foo",
					},
					Annotations: map[string]string{
						"jim": "foo",
					},
				},
			})

			packageRepo.CreatePackageReturns(repositories.PackageRecord{
				Type:        "bits",
				AppGUID:     appGUID,
				SpaceGUID:   spaceGUID,
				GUID:        packageGUID,
				State:       "AWAITING_UPLOAD",
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
				Labels:      map[string]string{"bob": "foo"},
				Annotations: map[string]string{"jim": "foo"},
			}, nil)

			appRepo.GetAppReturns(repositories.AppRecord{
				SpaceGUID: spaceGUID,
				GUID:      appGUID,
				EtcdUID:   appUID,
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", "/v3/packages", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("creates a CFPackage", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(packageRepo.CreatePackageCallCount()).To(Equal(1))
			_, actualAuthInfo, actualCreate := packageRepo.CreatePackageArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualCreate).To(Equal(repositories.CreatePackageMessage{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				Metadata: repositories.Metadata{
					Labels: map[string]string{
						"bob": "foo",
					},
					Annotations: map[string]string{
						"jim": "foo",
					},
				},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", packageGUID),
				MatchJSONPath("$.state", "AWAITING_UPLOAD"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
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

		When("the request JSON is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "test-error"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("test-error")
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

	Describe("the PATCH /v3/packages/:guid endpoint", func() {
		BeforeEach(func() {
			packageGUID = generateGUID("package")

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.PackageUpdate{
				Metadata: payloads.MetadataPatch{
					Labels: map[string]*string{
						"bob": tools.PtrTo("foo"),
					},
					Annotations: map[string]*string{
						"jim": tools.PtrTo("foo"),
					},
				},
			})

			packageRepo.UpdatePackageReturns(repositories.PackageRecord{
				Type:        "bits",
				AppGUID:     appGUID,
				SpaceGUID:   spaceGUID,
				GUID:        packageGUID,
				State:       "AWAITING_UPLOAD",
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
				Labels:      map[string]string{"bob": "foo"},
				Annotations: map[string]string{"jim": "foo"},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "PATCH", "/v3/packages/"+packageGUID, strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("validates the request payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
		})

		When("the request payload validation fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("req-invalid"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		It("returns Content-Type as JSON in header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
		})

		It("Updates a CFPackage", func() {
			Expect(packageRepo.UpdatePackageCallCount()).To(Equal(1))
			_, actualAuthInfo, actualUpdate := packageRepo.UpdatePackageArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualUpdate).To(Equal(repositories.UpdatePackageMessage{
				GUID: packageGUID,
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"bob": tools.PtrTo("foo"),
					},
					Annotations: map[string]*string{
						"jim": tools.PtrTo("foo"),
					},
				},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", packageGUID),
				MatchJSONPath("$.state", "AWAITING_UPLOAD"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("updating the package fails", func() {
			BeforeEach(func() {
				packageRepo.UpdatePackageReturns(repositories.PackageRecord{}, errors.New("boom"))
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
				ImageRef:  "registry.repo/foo",
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

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("uploads the package", func() {
			Expect(packageRepo.GetPackageCallCount()).To(Equal(1))

			_, actualAuthInfo, actualPackageGUID := packageRepo.GetPackageArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualPackageGUID).To(Equal(packageGUID))

			Expect(imageRepo.UploadSourceImageCallCount()).To(Equal(1))
			_, actualAuthInfo, repoRef, srcFile, actualSpaceGUID, actualTags := imageRepo.UploadSourceImageArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(repoRef).To(Equal("registry.repo/foo"))
			actualSrcContents, err := io.ReadAll(srcFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actualSrcContents)).To(Equal("the-src-file-contents"))
			Expect(actualSpaceGUID).To(Equal(spaceGUID))
			Expect(actualTags).To(HaveLen(1))
			Expect(actualTags[0]).To(Equal(packageGUID))

			Expect(packageRepo.UpdatePackageSourceCallCount()).To(Equal(1))
			_, actualAuthInfo, message := packageRepo.UpdatePackageSourceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(message.GUID).To(Equal(packageGUID))
			Expect(message.ImageRef).To(Equal(imageRefWithDigest))
			Expect(message.RegistrySecretNames).To(ConsistOf(packageImagePullSecretNames))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAny(
				MatchJSONPath("$.guid", packageGUID),
				MatchJSONPath("$.state", "READY"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
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
				expectNotFoundError("Package")
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
				Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
			})

			It("returns a CF API formatted Error response", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
					"errors": [
						{
							"title": "CF-PackageBitsAlreadyUploaded",
							"detail": "Bits may be uploaded only once. Create a new package to upload different bits.",
							"code": 150004
						}
					]
				}`)))
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
			queryString = "?not=used"

			payload := payloads.PackageListDroplets{}
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payload)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/packages/"+packageGUID+"/droplets"+queryString, nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("fetches the droplet", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix(queryString))

			Expect(packageRepo.GetPackageCallCount()).To(Equal(1))

			_, _, actualPackageGUID := packageRepo.GetPackageArgsForCall(0)
			Expect(actualPackageGUID).To(Equal(packageGUID))

			Expect(dropletRepo.ListDropletsCallCount()).To(Equal(1))

			_, _, dropletListMessage := dropletRepo.ListDropletsArgsForCall(0)
			Expect(dropletListMessage).To(Equal(repositories.ListDropletsMessage{
				PackageGUIDs: []string{packageGUID},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/packages/"+packageGUID+"/droplets?not=used"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].guid", Equal(dropletGUID)),
				MatchJSONPath("$.resources[0].state", Equal("STAGED")),
			)))
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

		When("the request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
