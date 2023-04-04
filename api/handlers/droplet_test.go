package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Droplet", func() {
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

		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())
		apiHandler := NewDroplet(
			*serverURL,
			dropletRepo,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/droplet/:guid endpoint", func() {
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
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				}, nil)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			})

			It("fetches the right droplet", func() {
				Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))

				_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
				Expect(actualDropletGUID).To(Equal(dropletGUID))
			})

			It("returns the droplet in the response", func() {
				Expect(rr.Body.Bytes()).To(MatchJSONPath("$.guid", dropletGUID))
				Expect(rr.Body.Bytes()).To(MatchJSONPath("$.process_types.rake", "bundle exec rake"))
			})
		})

		When("access to the droplet is forbidden", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewForbiddenError(nil, repositories.DropletResourceType))
			})

			It("returns a Not Found error", func() {
				expectNotFoundError("Droplet not found")
			})
		})

		When("there is some other error fetching the droplet", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/droplet/:guid endpoint", func() {
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
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			}, nil)

			dropletRepo.UpdateDropletReturns(repositories.DropletRecord{
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
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"bar": "baz",
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader(`{
				  "metadata": {
					"labels": {
					  "foo": "bar"
					},
					"annotations": {
					  "bar": "baz"
					}
				  }
				}`))
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("has the correct response type", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})

			It("returns Content-Type as JSON in header", func() {
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			})

			It("calls update droplet with the correct payload", func() {
				Expect(dropletRepo.UpdateDropletCallCount()).To(Equal(1))
				_, _, actualUpdate := dropletRepo.UpdateDropletArgsForCall(0)

				Expect(actualUpdate.GUID).To(Equal(dropletGUID))
				Expect(actualUpdate.MetadataPatch).To(Equal(repositories.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				}))
			})

			It("returns the droplet in the response", func() {
				Expect(rr.Body.Bytes()).To(MatchJSONPath("$.guid", dropletGUID))
				Expect(rr.Body.Bytes()).To(MatchJSONPath("$.process_types.rake", "bundle exec rake"))
			})
		})

		When("the payload cannot be decoded", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader(`{"one": "two"}`))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("invalid request body: json: unknown field \"one\"")
			})
		})

		When("a label is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader(`{
					  "metadata": {
						"labels": {
						  "cloudfoundry.org/test": "production"
					    }
				      }
					}`))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})

			When("the prefix is a subdomain of cloudfoundry.org", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader(`{
					  "metadata": {
						"labels": {
						  "korifi.cloudfoundry.org/test": "production"
					    }
			          }
					}`))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})
		})

		When("an annotation is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader(`{
					  "metadata": {
						"annotations": {
						  "cloudfoundry.org/test": "there"
						}
					  }
					}`))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})

				When("the prefix is a subdomain of cloudfoundry.org", func() {
					BeforeEach(func() {
						var err error
						req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader(`{
						  "metadata": {
							"annotations": {
							  "korifi.cloudfoundry.org/test": "there"
							}
						  }
						}`))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns an unprocessable entity error", func() {
						expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
					})
				})
			})
		})

		When("the droplet repo returns an error", func() {
			BeforeEach(func() {
				dropletRepo.UpdateDropletReturns(repositories.DropletRecord{}, errors.New("update-droplet-error"))
			})
			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user is not authorized to get droplets", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewForbiddenError(nil, "CFDroplet"))
			})

			It("returns 404 NotFound", func() {
				expectNotFoundError("CFDroplet not found")
			})
		})
	})
})
