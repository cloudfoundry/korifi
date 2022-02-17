package apis_test

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
)

var _ = Describe("DropletHandler", func() {
	const (
		testDropletHandlerLoggerName = "TestDropletHandler"
	)
	Describe("the GET /v3/droplet/:guid endpoint", func() {
		const (
			appGUID     = "test-app-guid"
			packageGUID = "test-package-guid"
			dropletGUID = "test-build-guid" // same as build guid

			createdAt = "1906-04-18T13:12:00Z"
			updatedAt = "1906-04-18T13:12:01Z"
		)
		var (
			dropletRepo *fake.CFDropletRepository
			req         *http.Request
		)

		BeforeEach(func() {
			dropletRepo = new(fake.CFDropletRepository)
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/droplets/"+dropletGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			dropletHandler := NewDropletHandler(
				logf.Log.WithName(testDropletHandlerLoggerName),
				*serverURL,
				dropletRepo,
			)
			dropletHandler.RegisterRoutes(router)
		})

		When("build staging is successful", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{
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
						"rake": "bundle exec rake",
						"web":  "bundle exec rackup config.ru -p $PORT",
					},
					AppGUID:     appGUID,
					PackageGUID: packageGUID,
				}, nil)
				router.ServeHTTP(rr, req)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("fetches the right droplet", func() {
				Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))

				_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
				Expect(actualDropletGUID).To(Equal(dropletGUID))
			})

			It("returns the droplet in the response", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
					  "guid": "`+dropletGUID+`",
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
						"rake": "bundle exec rake",
						"web": "bundle exec rackup config.ru -p $PORT"
					  },
					  "checksum": null,
					  "buildpacks": [],
					  "stack": "cflinuxfs3",
					  "image": null,
					  "created_at": "`+createdAt+`",
					  "updated_at": "`+updatedAt+`",
					  "relationships": {
						"app": {
						  "data": {
							"guid": "`+appGUID+`"
						  }
						}
					  },
					  "links": {
						"self": {
						  "href": "`+defaultServerURI("/v3/droplets/", dropletGUID)+`"
						},
						"package": {
						  "href": "`+defaultServerURI("/v3/packages/", packageGUID)+`"
						},
						"app": {
						  "href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						},
						"assign_current_droplet": {
						  "href": "`+defaultServerURI("/v3/apps/", appGUID, "/relationships/current_droplet")+`",
						  "method": "PATCH"
						  },
						"download": null
					  },
					  "metadata": {
						"labels": {},
						"annotations": {}
					  }
					}`), "Response body matches response:")
			})
		})

		When("the droplet cannot be found", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, repositories.NewNotFoundError(repositories.DropletResourceType, nil))
				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectNotFoundError("Droplet not found")
			})
		})

		When("access to the droplet is forbidden", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, repositories.NewForbiddenError("Droplet", nil))
				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectNotFoundError("Droplet not found")
			})
		})

		When("there is some other error fetching the droplet", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("unknown!"))

				router.ServeHTTP(rr, req)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
