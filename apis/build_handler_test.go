package apis_test

import (
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	testBuildHandlerLoggerName = "TestBuildHandler"
)

func TestBuild(t *testing.T) {
	spec.Run(t, "the GET /v3/builds/{guid} endpoint", testBuildGetHandler, spec.Report(report.Terminal{}))
	spec.Run(t, "the POST /v3/builds endpoint", testBuildCreateHandler, spec.Report(report.Terminal{}))
}

func testBuildGetHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		appGUID     = "test-app-guid"
		packageGUID = "test-package-guid"
		buildGUID   = "test-build-guid"

		stagingMem  = 1024
		stagingDisk = 2048

		createdAt = "1906-04-18T13:12:00Z"
		updatedAt = "1906-04-18T13:12:01Z"
	)

	var (
		rr            *httptest.ResponseRecorder
		req           *http.Request
		buildRepo     *fake.CFBuildRepository
		clientBuilder *fake.ClientBuilder
		router        *mux.Router
	)

	getRR := func() *httptest.ResponseRecorder { return rr }

	// set up happy path defaults
	it.Before(func() {
		buildRepo = new(fake.CFBuildRepository)
		buildRepo.FetchBuildReturns(repositories.BuildRecord{
			GUID:            buildGUID,
			State:           "STAGING",
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			StagingMemoryMB: stagingMem,
			StagingDiskMB:   stagingDisk,
			Lifecycle: repositories.Lifecycle{
				Type: "buildpack",
				Data: repositories.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
			PackageGUID: packageGUID,
			AppGUID:     appGUID,
		}, nil)

		var err error
		req, err = http.NewRequest("GET", "/v3/builds/"+buildGUID, nil)
		g.Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		router = mux.NewRouter()
		clientBuilder = new(fake.ClientBuilder)

		buildHandler := NewBuildHandler(
			logf.Log.WithName(testBuildHandlerLoggerName),
			defaultServerURL,
			buildRepo,
			new(fake.CFPackageRepository),
			clientBuilder.Spy,
			&rest.Config{},
		)
		buildHandler.RegisterRoutes(router)
	})

	when("on the happy path", func() {
		when("build staging is not complete", func() {
			it.Before(func() {
				router.ServeHTTP(rr, req)
			})

			it("returns status 200 OK", func() {
				g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			it("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			it("returns the Build in the response", func() {
				g.Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "STAGING",
					"staging_memory_in_mb": `+fmt.Sprint(stagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(stagingDisk)+`,
					"error": null,
					"lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": null,
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						}
					}
				}`), "Response body matches response:")
			})
		})
		when("build staging is successful", func() {

			it.Before(func() {
				buildRepo.FetchBuildReturns(repositories.BuildRecord{
					GUID:            buildGUID,
					State:           "STAGED",
					CreatedAt:       createdAt,
					UpdatedAt:       updatedAt,
					StagingMemoryMB: stagingMem,
					StagingDiskMB:   stagingDisk,
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
					PackageGUID: packageGUID,
					DropletGUID: buildGUID,
					AppGUID:     appGUID,
				}, nil)
				router.ServeHTTP(rr, req)
			})

			it("returns status 200 OK", func() {
				g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			it("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			it("returns the Build in the response", func() {
				g.Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "STAGED",
					"staging_memory_in_mb": `+fmt.Sprint(stagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(stagingDisk)+`,
					"error": null,
					"lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": {
						"guid": "`+buildGUID+`"
					},
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						},
						"droplet": {
							"href": "`+defaultServerURI("/v3/droplets/", buildGUID)+`"
						}
					}
				}`), "Response body matches response:")
			})
		})
		when("build staging fails", func() {
			const (
				stagingErrorMsg = "StagingError: something went wrong during staging"
			)
			it.Before(func() {
				buildRepo.FetchBuildReturns(repositories.BuildRecord{
					GUID:            buildGUID,
					State:           "FAILED",
					CreatedAt:       createdAt,
					UpdatedAt:       updatedAt,
					StagingErrorMsg: stagingErrorMsg,
					StagingMemoryMB: stagingMem,
					StagingDiskMB:   stagingDisk,
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
					PackageGUID: packageGUID,
					DropletGUID: "",
					AppGUID:     appGUID,
				}, nil)
				router.ServeHTTP(rr, req)
			})

			it("returns status 200 OK", func() {
				g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			it("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			it("returns the Build in the response", func() {
				g.Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "FAILED",
					"staging_memory_in_mb": `+fmt.Sprint(stagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(stagingDisk)+`,
					"error": "`+stagingErrorMsg+`",
					"lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": null,
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						}
					}
				}`), "Make sure there is no droplet and error is surfaced from record")
			})
		})
	})

	when("building the k8s client errors", func() {
		it.Before(func() {
			clientBuilder.Returns(nil, errors.New("boom"))
			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("the build cannot be found", func() {
		it.Before(func() {
			buildRepo.FetchBuildReturns(repositories.BuildRecord{}, repositories.NotFoundError{})

			router.ServeHTTP(rr, req)
		})

		itRespondsWithNotFound(it, g, "Build not found", getRR)
	})

	when("there is some other error fetching the build", func() {
		it.Before(func() {
			buildRepo.FetchBuildReturns(repositories.BuildRecord{}, errors.New("unknown!"))

			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

}

func testBuildCreateHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var (
		rr            *httptest.ResponseRecorder
		packageRepo   *fake.CFPackageRepository
		buildRepo     *fake.CFBuildRepository
		clientBuilder *fake.ClientBuilder
		router        *mux.Router
	)

	makePostRequest := func(body string) {
		req, err := http.NewRequest("POST", "/v3/builds", strings.NewReader(body))
		g.Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	}

	const (
		packageGUID = "the-package-guid"
		appGUID     = "the-app-guid"
		buildGUID   = "test-build-guid"

		expectedStagingMem     = 1024
		expectedStagingDisk    = 1024
		expectedLifecycleType  = "buildpack"
		expectedLifecycleStack = "cflinuxfs3"
		spaceGUID              = "the-space-guid"
		validBody              = `{
			"package": {
				"guid": "` + packageGUID + `"
        	}
		}`
		createdAt = "1906-04-18T13:12:00Z"
		updatedAt = "1906-04-18T13:12:01Z"
	)

	getRR := func() *httptest.ResponseRecorder { return rr }

	it.Before(func() {
		rr = httptest.NewRecorder()
		router = mux.NewRouter()

		packageRepo = new(fake.CFPackageRepository)
		packageRepo.FetchPackageReturns(repositories.PackageRecord{
			Type:      "bits",
			AppGUID:   appGUID,
			SpaceGUID: spaceGUID,
			GUID:      packageGUID,
			State:     "READY",
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}, nil)

		buildRepo = new(fake.CFBuildRepository)
		buildRepo.CreateBuildReturns(repositories.BuildRecord{
			GUID:            buildGUID,
			State:           "STAGING",
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			StagingMemoryMB: expectedStagingMem,
			StagingDiskMB:   expectedStagingDisk,
			Lifecycle: repositories.Lifecycle{
				Type: expectedLifecycleType,
				Data: repositories.LifecycleData{
					Buildpacks: []string{},
					Stack:      expectedLifecycleStack,
				},
			},
			PackageGUID: packageGUID,
			AppGUID:     appGUID,
		}, nil)

		clientBuilder = new(fake.ClientBuilder)
		buildHandler := NewBuildHandler(
			logf.Log.WithName(testBuildHandlerLoggerName),
			defaultServerURL,
			buildRepo,
			packageRepo,
			clientBuilder.Spy,
			&rest.Config{},
		)
		buildHandler.RegisterRoutes(router)
	})

	when("on the happy path", func() {
		it.Before(func() {
			makePostRequest(validBody)
		})

		it("returns status 201", func() {
			g.Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("configures the client", func() {
			g.Expect(clientBuilder.CallCount()).To(Equal(1))
		})

		when("examining the BuildCreate message", func() {
			var (
				actualCreate repositories.BuildCreateMessage
			)
			it.Before(func() {
				g.Expect(buildRepo.CreateBuildCallCount()).To(Equal(1), "buildRepo CreateBuild was not called")
				_, _, actualCreate = buildRepo.CreateBuildArgsForCall(0)
			})
			it("has the same SpaceGUID as the package", func() {
				g.Expect(actualCreate.SpaceGUID).To(Equal(spaceGUID))
			})
			it("has the same AppGUID as the package", func() {
				g.Expect(actualCreate.AppGUID).To(Equal(appGUID))
			})
			it("has the same PackageGUID as the request", func() {
				g.Expect(actualCreate.PackageGUID).To(Equal(packageGUID))
			})
			it("fills in values for StagingMemoryMB", func() {
				g.Expect(actualCreate.StagingMemoryMB).To(Equal(expectedStagingMem))

			})
			it("fills in values for StagingDiskMB", func() {
				g.Expect(actualCreate.StagingDiskMB).To(Equal(expectedStagingDisk))
			})
			it("fills in values for Lifecycle", func() {
				g.Expect(actualCreate.Lifecycle.Type).To(Equal(expectedLifecycleType))
				g.Expect(actualCreate.Lifecycle.Data.Buildpacks).To(Equal([]string{}))
				g.Expect(actualCreate.Lifecycle.Data.Stack).To(Equal(expectedLifecycleStack))
			})
		})

		it("returns the Build in the response", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "STAGING",
					"staging_memory_in_mb": `+fmt.Sprint(expectedStagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(expectedStagingDisk)+`,
					"error": null,
					"lifecycle": {
						"type": "`+expectedLifecycleType+`",
						"data": {
							"buildpacks": [],
							"stack": "`+expectedLifecycleStack+`"
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": null,
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						}
					}
				}`), "Response body matches response:")
		})
	})

	when("the package doesn't exist", func() {
		it.Before(func() {
			packageRepo.FetchPackageReturns(repositories.PackageRecord{}, repositories.NotFoundError{})

			makePostRequest(validBody)
		})

		it("returns status 422", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("responds with error code", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
					"errors": [
						{
							"code": 10008,
							"title": "CF-UnprocessableEntity",
							"detail": "Unable to use package. Ensure that the package exists and you have access to it."
						}
					]
				}`))
		})

		it("doesn't create a build", func() {
			g.Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
		})
	})

	when("the package exists check returns an error", func() {
		it.Before(func() {
			packageRepo.FetchPackageReturns(repositories.PackageRecord{}, errors.New("boom"))

			makePostRequest(validBody)
		})

		itRespondsWithUnknownError(it, g, getRR)

		it("doesn't create a build", func() {
			g.Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
		})
	})

	when("building the k8s client errors", func() {
		it.Before(func() {
			clientBuilder.Returns(nil, errors.New("boom"))
			makePostRequest(validBody)
		})

		itRespondsWithUnknownError(it, g, getRR)

		it("doesn't create a Build", func() {
			g.Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
		})
	})

	when("creating the build in the repo errors", func() {
		it.Before(func() {
			buildRepo.CreateBuildReturns(repositories.BuildRecord{}, errors.New("boom"))
			makePostRequest(validBody)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("the JSON body is invalid", func() {
		it.Before(func() {
			makePostRequest(`{`)
		})

		it("returns a status 400 Bad Request ", func() {
			g.Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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

}
