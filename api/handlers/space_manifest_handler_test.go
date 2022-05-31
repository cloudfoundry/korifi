package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("SpaceManifestHandler", func() {
	var (
		applyManifestAction *fake.ApplyManifestAction
		spaceRepo           *fake.CFSpaceRepository
		req                 *http.Request
		defaultDomainName   string
		requestBody         *strings.Reader
	)

	BeforeEach(func() {
		applyManifestAction = new(fake.ApplyManifestAction)
		spaceRepo = new(fake.CFSpaceRepository)
		defaultDomainName = "apps.example.org"

		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewSpaceManifestHandler(
			*serverURL,
			defaultDomainName,
			applyManifestAction.Spy,
			spaceRepo,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)
	})

	Describe("POST /v3/spaces/{spaceGUID}/actions/apply_manifest", func() {
		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", requestBody)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/x-yaml")

			router.ServeHTTP(rr, req)
		})

		When("the manifest is valid", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  default-route: true
                  memory: 128M
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
				Expect(applyManifestAction.CallCount()).To(Equal(1))
				_, actualAuthInfo, _, _, _ := applyManifestAction.ArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("passes the parsed manifest to the action", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(1))
				_, _, _, _, payload := applyManifestAction.ArgsForCall(0)

				Expect(payload.Applications).To(HaveLen(1))
				Expect(payload.Applications[0].Name).To(Equal("app1"))
				Expect(payload.Applications[0].DefaultRoute).To(BeTrue())
				Expect(payload.Applications[0].Memory).To(PointTo(Equal("128M")))

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
				Expect(applyManifestAction.CallCount()).To(Equal(1))
				_, actualAuthInfo, _, _, _ := applyManifestAction.ArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})
		})

		When("the manifest contains multiple apps", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                - name: app2
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Applications must contain at maximum 1 item")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("the application name is missing", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - {}
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Name is a required field")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("the application memory is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: test-app
                  memory: 0
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].Memory' Error:Field validation for 'Memory' failed on the 'megabytestring' tag")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("the application route is invalid", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: my-app
                  routes:
                  - route: not-a-uri?
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError(`"not-a-uri?" is not a valid route URI`)
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("the application process instance count is negative", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    instances: -1
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Instances must be 0 or greater")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("the application process disk is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    disk_quota: 0
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].Processes[0].DiskQuota' Error:Field validation for 'DiskQuota' failed on the 'megabytestring' tag")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("the application process memory is not a positive integer", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    memory: 0
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("Key: 'Manifest.Applications[0].Processes[0].Memory' Error:Field validation for 'Memory' failed on the 'megabytestring' tag")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("a process's health-check-type is invalid", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: test-app
                  processes:
                  - type: web
                    health-check-type: bogus-type
                `)
			})

			It("response with an unprocessable entity error", func() {
				expectUnprocessableEntityError("HealthCheckType must be one of [none process port http]")
			})

			It("doesn't call applyManifestAction", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("applying the manifest errors", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                `)
				applyManifestAction.Returns(errors.New("boom"))
			})

			It("respond with Unknown Error", func() {
				expectUnknownError()
			})
		})

		When("applying the manifest errors with NotFoundErr", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                `)
				applyManifestAction.Returns(apierrors.NewNotFoundError(errors.New("can't find"), repositories.DomainResourceType))
			})

			It("respond with NotFoundErr", func() {
				expectNotFoundError("Domain")
			})
		})
	})

	Describe("POST /v3/spaces/{spaceGUID}/manifest_diff", func() {
		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
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
