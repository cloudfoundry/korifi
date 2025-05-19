package handlers_test

import (
	"errors"
	"net/http"
	"strings"
	"time"

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
	var (
		createdAt = time.UnixMilli(2000)
		updatedAt = tools.PtrTo(time.UnixMilli(2000))

		appGUID      string
		packageGUID  string
		dropletGUID  string
		dropletGUID2 string

		requestValidator *fake.RequestValidator
		dropletRepo      *fake.CFDropletRepository
		req              *http.Request
		err              error
		reqPath          string
		reqMethod        string
	)

	BeforeEach(func() {
		dropletRepo = new(fake.CFDropletRepository)
		requestValidator = new(fake.RequestValidator)

		appGUID = "test-app-guid"
		packageGUID = "test-package-guid"
		dropletGUID = "test-build-guid" // same as build guid
		dropletGUID2 = "test-build-guid-2"

		apiHandler := NewDroplet(
			*serverURL,
			dropletRepo,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
		reqPath = "/v3/droplets"
		reqMethod = http.MethodGet
	})

	JustBeforeEach(func() {
		req, err = http.NewRequestWithContext(ctx, reqMethod, reqPath, strings.NewReader("the-json-body"))
		Expect(err).NotTo(HaveOccurred())
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/droplet/:guid endpoint", func() {
		BeforeEach(func() {
			reqPath += "/" + dropletGUID
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

	Describe("the GET /v3/droplets endpoint", func() {
		BeforeEach(func() {
			dropletRepo.ListDropletsReturns([]repositories.DropletRecord{
				{
					GUID:  dropletGUID,
					State: "STAGED",
					ProcessTypes: map[string]string{
						"rake": "bundle exec rake",
						"web":  "bundle exec rackup config.ru -p $PORT",
					},
				},
				{
					GUID:  dropletGUID2,
					State: "STAGED",
				},
			}, nil)
		})

		It("lists the droplets", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(dropletRepo.ListDropletsCallCount()).To(Equal(1))
			_, _, message := dropletRepo.ListDropletsArgsForCall(0)
			Expect(message.AppGUIDs).To(BeEmpty())
			Expect(message.SpaceGUIDs).To(BeEmpty())
			Expect(message.PackageGUIDs).To(BeEmpty())
			Expect(message.GUIDs).To(BeEmpty())

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(2)),
				MatchJSONPath("$.resources[0].guid", dropletGUID),
				MatchJSONPath("$.resources[0].process_types.rake", "bundle exec rake"),
				MatchJSONPath("$.resources[1].guid", dropletGUID2),
			)))
		})

		When("the droplet repo returns an error", func() {
			BeforeEach(func() {
				dropletRepo.ListDropletsReturns([]repositories.DropletRecord{}, errors.New("update-droplet-error"))
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

			payload = &payloads.DropletUpdate{
				Metadata: payloads.MetadataPatch{
					Labels:      map[string]*string{"foo": tools.PtrTo("bar")},
					Annotations: map[string]*string{"bar": tools.PtrTo("baz")},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)
			reqPath += "/" + dropletGUID
			reqMethod = http.MethodPatch
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
