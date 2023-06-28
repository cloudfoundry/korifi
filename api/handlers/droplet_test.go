package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
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
		requestValidator *fake.RequestValidator
		dropletRepo      *fake.CFDropletRepository
		req              *http.Request
	)

	BeforeEach(func() {
		dropletRepo = new(fake.CFDropletRepository)
		var err error
		req, err = http.NewRequestWithContext(ctx, "GET", "/v3/droplets/"+dropletGUID, nil)
		Expect(err).NotTo(HaveOccurred())

		requestValidator = new(fake.RequestValidator)

		apiHandler := NewDroplet(
			*serverURL,
			dropletRepo,
			requestValidator,
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

			It("returns the droplet", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
				_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
				Expect(actualDropletGUID).To(Equal(dropletGUID))

				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.guid", dropletGUID),
					MatchJSONPath("$.process_types.rake", "bundle exec rake"),
				)))
			})
		})

		When("access to the droplet is forbidden", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewForbiddenError(nil, repositories.DropletResourceType))
			})

			It("returns a Not Found error", func() {
				expectNotFoundError("Droplet")
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
		var payload *payloads.DropletUpdate

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
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/droplets/"+dropletGUID, strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())

			payload = &payloads.DropletUpdate{
				Metadata: payloads.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		It("updates the droplet", func() {
			Expect(dropletRepo.UpdateDropletCallCount()).To(Equal(1))
			_, _, actualUpdate := dropletRepo.UpdateDropletArgsForCall(0)
			Expect(actualUpdate.GUID).To(Equal(dropletGUID))
			Expect(actualUpdate.MetadataPatch).To(Equal(repositories.MetadataPatch{
				Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
				Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", dropletGUID),
				MatchJSONPath("$.process_types.rake", "bundle exec rake"),
			)))
		})

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("validation error")
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
				expectNotFoundError("CFDroplet")
			})
		})
	})
})
