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
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("SpaceManifest", func() {
	var (
		manifestApplier  *fake.ManifestApplier
		spaceRepo        *fake.CFSpaceRepository
		requestValidator *fake.RequestValidator
		requestMethod    string
		requestPath      string
	)

	BeforeEach(func() {
		requestMethod = "POST"
		requestPath = ""

		manifestApplier = new(fake.ManifestApplier)
		spaceRepo = new(fake.CFSpaceRepository)
		requestValidator = new(fake.RequestValidator)

		apiHandler := NewSpaceManifest(
			*serverURL,
			manifestApplier,
			spaceRepo,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader("the-yaml-body"))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Add("Content-type", "application/x-yaml")
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/spaces/{spaceGUID}/actions/apply_manifest", func() {
		BeforeEach(func() {
			requestPath = "/v3/spaces/test-space-guid/actions/apply_manifest"
			requestValidator.DecodeAndValidateYAMLPayloadStub = decodeAndValidatePayloadStub(&payloads.Manifest{
				Version: 1,
				Applications: []payloads.ManifestApplication{{
					Name:         "app1",
					DefaultRoute: true,
					Memory:       tools.PtrTo("128M"),
					DiskQuota:    tools.PtrTo("256M"),
					Processes: []payloads.ManifestApplicationProcess{{
						Type:                         "web",
						Command:                      tools.PtrTo("start-web.sh"),
						DiskQuota:                    tools.PtrTo("512M"),
						HealthCheckHTTPEndpoint:      tools.PtrTo("/healthcheck"),
						HealthCheckInvocationTimeout: tools.PtrTo[int64](5),
						HealthCheckType:              tools.PtrTo("http"),
						Instances:                    tools.PtrTo(1),
						Memory:                       tools.PtrTo("256M"),
						Timeout:                      tools.PtrTo[int64](10),
					}},
				}},
			})
		})

		It("applies the manifest", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", ContainSubstring("space.apply_manifest~"+spaceGUID)))

			Expect(requestValidator.DecodeAndValidateYAMLPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateYAMLPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-yaml-body"))

			Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
			_, actualAuthInfo, _, payload := manifestApplier.ApplyArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

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

		When("the manifest is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateYAMLPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/spaces/{spaceGUID}/manifest_diff", func() {
		BeforeEach(func() {
			requestPath = "/v3/spaces/test-space-guid/manifest_diff"
		})

		It("returns 202 with an empty diff", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
				"diff": []
			}`)))
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
