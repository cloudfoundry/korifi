package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("SpaceManifest", func() {
	var (
		manifestApplier *fake.ManifestApplier
		spaceRepo       *fake.CFSpaceRepository
		req             *http.Request
		requestBody     *strings.Reader
	)

	BeforeEach(func() {
		manifestApplier = new(fake.ManifestApplier)
		spaceRepo = new(fake.CFSpaceRepository)

		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewSpaceManifest(
			*serverURL,
			manifestApplier,
			spaceRepo,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("POST /v3/spaces/{spaceGUID}/actions/apply_manifest", func() {
		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", requestBody)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/x-yaml")

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		When("the manifest is valid", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  default-route: true
                  memory: 128M
                  disk_quota: 256M
                  processes:
                  - type: web
                    command: start-web.sh
                    disk_quota: 512M
                    health-check-http-endpoint: /healthcheck
                    health-check-invocation-timeout: 5
                    health-check-type: http
                    instances: 1
                    memory: 256M
                    timeout: 10
                `)
			})

			It("returns 202 with a Location header", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))

				Expect(rr).To(HaveHTTPHeaderWithValue("Location", ContainSubstring("space.apply_manifest~"+spaceGUID)))
			})

			It("calls applyManifestAction and passes it the authInfo from the context", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ := manifestApplier.ApplyArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("passes the parsed manifest to the action", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
				_, _, _, payload := manifestApplier.ApplyArgsForCall(0)

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
		})

		When("the manifest is invalid", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  default-route: true
                  memory: "128"
                  disk_quota: 256M
                  processes:
                  - type: web
                    command: start-web.sh
                    disk_quota: 512M
                    health-check-http-endpoint: /healthcheck
                    health-check-invocation-timeout: 5
                    health-check-type: httpppp
                    instances: 1
                    memory: 256M
                    timeout: 1
                  routes:
                  - route: "ssh://1.2.3.4"
                `)
			})

			It("response with an unprocessable entity error listing all the errors", func() {
				expectUnprocessableEntityError("applications[0].memory must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB), applications[0].processes[0].health-check-type must be a valid value, applications[0].routes[0].route is not a valid route")
			})
		})

		When("the manifest contains unsupported fields", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  metadata:
                    annotations:
                      contact: "bob@example.com jane@example.com"
                    labels:
                      sensitive: true
                `)
			})

			It("calls applyManifestAction and passes it the authInfo from the context", func() {
				Expect(manifestApplier.ApplyCallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ := manifestApplier.ApplyArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})
		})

		When("random-route and default-route flags are both set", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  default-route: true
                  random-route: true
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("applications[0].default-route and random-route may not be used together")
			})
		})

		When("app disk-quota and app disk_quota are both set", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  disk-quota: 128M
                  disk_quota: 128M
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("applications[0].disk_quota and disk-quota may not be used together")
			})
		})

		When("process disk-quota and process disk_quota are both set", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  processes:
                  - type: foo
                    disk-quota: 128M
                    disk_quota: 128M
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("applications[0].processes[0].disk_quota and disk-quota may not be used together")
			})
		})
	})

	Describe("POST /v3/spaces/{spaceGUID}/manifest_diff", func() {
		JustBeforeEach(func() {
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		When("the space exists", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "POST", "/v3/spaces/"+spaceGUID+"/manifest_diff", strings.NewReader(`---
					version: 1
					applications:
					  - name: app1
				`))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-type", "application/x-yaml")
			})

			It("returns 202 with an empty diff", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                	"diff": []
            	}`)))
			})
		})

		When("getting the space errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("foo"))
				var err error
				req, err = http.NewRequestWithContext(ctx, "POST", "/v3/spaces/fake-space-guid/manifest_diff", strings.NewReader(`---
					version: 1
					applications:
					- name: app1
				`))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-type", "application/x-yaml")
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("getting the space is forbidden", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(errors.New("foo"), repositories.SpaceResourceType))
				var err error
				req, err = http.NewRequestWithContext(ctx, "POST", "/v3/spaces/fake-space-guid/manifest_diff", strings.NewReader(`---
					version: 1
					applications:
					- name: app1
				`))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-type", "application/x-yaml")
			})

			It("returns an error", func() {
				expectNotFoundError("Space")
			})
		})
	})
})
