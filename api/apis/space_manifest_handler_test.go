package apis_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"errors"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
)

var _ = Describe("SpaceManifestHandler", func() {
	var (
		clientBuilder       *fake.ClientBuilder
		applyManifestAction *fake.ApplyManifestAction
		spaceRepo           *fake.CFSpaceRepository
	)

	BeforeEach(func() {
		clientBuilder = new(fake.ClientBuilder)
		applyManifestAction = new(fake.ApplyManifestAction)
		spaceRepo = new(fake.CFSpaceRepository)

		now := time.Unix(1631892190, 0)
		spaceRepo.FetchSpacesReturns([]repositories.SpaceRecord{
			{
				Name:             "my-test-space",
				GUID:             spaceGUID,
				OrganizationGUID: "org-guid-1",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				Name:             "bob",
				GUID:             "b-o-b",
				OrganizationGUID: "org-guid-2",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		}, nil)

		apiHandler := NewSpaceManifestHandler(
			logf.Log.WithName("testSpaceManifestHandler"),
			*serverURL,
			applyManifestAction.Spy,
			spaceRepo,
			clientBuilder.Spy,
			&rest.Config{},
		)
		apiHandler.RegisterRoutes(router)
	})

	Describe("POST /v3/spaces/{spaceGUID}/actions/apply_manifest", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", strings.NewReader(`---
                version: 1
                applications:
                  - name: app1
            `))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/x-yaml")
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("unsupported fields are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", strings.NewReader(`---
                version: 1
                applications:
                - name: app1
                  buildpacks:
                  - ruby_buildpack
                  - java_buildpack
                  env:
                    VAR1: value1
                    VAR2: value2
                  routes:
                  - route: route.example.com
                  - route: another-route.example.com
                    protocol: http2
                  services:
                  - my-service1
                  - my-service2
                  - name: my-service-with-arbitrary-params
                    binding_name: my-binding
                    parameters:
                      key1: value1
                      key2: value2
                  stack: cflinuxfs3
                  metadata:
                    annotations:
                      contact: "bob@example.com jane@example.com"
                    labels:
                      sensitive: true
                  processes:
                  - type: web
                    command: start-web.sh
                    disk_quota: 512M
                    health-check-http-endpoint: /healthcheck
                    health-check-type: http
                    health-check-invocation-timeout: 10
                    instances: 3
                    memory: 500M
                    timeout: 10
                  - type: worker
                    command: start-worker.sh
                    disk_quota: 1G
                    health-check-type: process
                    instances: 2
                    memory: 256M
                    timeout: 15 
            `))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-type", "application/x-yaml")
			})

			It("creates the record without erroring", func() {
				Expect(rr.Code).To(Equal(http.StatusAccepted))
				Expect(rr.Header().Get("Location")).To(Equal(defaultServerURI("/v3/jobs/sync-space.apply_manifest-", spaceGUID)))
				Expect(applyManifestAction.CallCount()).To(Equal(1))
			})
		})

		When("the manifest contains multiple apps", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", strings.NewReader(`---
                version: 1
                applications:
                  - name: app1
                  - name: app2
            `))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-type", "application/x-yaml")
			})

			It("responds 422", func() {
				expectUnprocessableEntityError("Applications must contain at maximum 1 item")
			})

			It("doesn't apply the manifest", func() {
				Expect(applyManifestAction.CallCount()).To(Equal(0))
			})
		})

		When("an invalid manifest is provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", strings.NewReader(`---
                version: 1
                applications:
                  - {}
            `))
				Expect(err).NotTo(HaveOccurred())
			})

			It("responds 422", func() {
				expectUnprocessableEntityError("Name is a required field")
			})
		})

		When("building the k8s client errors", func() {
			BeforeEach(func() {
				clientBuilder.Returns(nil, errors.New("boom"))
			})

			It("responds with Unknown Error", func() {
				expectUnknownError()
			})
		})

		When("applying the manifest errors", func() {
			BeforeEach(func() {
				applyManifestAction.Returns(errors.New("boom"))
			})

			It("respond with Unknown Error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/spaces/{spaceGUID}/manifest_diff", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("POST", "/v3/spaces/"+spaceGUID+"/manifest_diff", strings.NewReader(`---
				version: 1
				applications:
				  - name: app1
			`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/x-yaml")
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("the space exists", func() {
			It("returns 202 with an empty diff", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                	"diff": []
            	}`)))
			})
		})

		When("the space is not found", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("POST", "/v3/spaces/"+"fake-space-guid"+"/manifest_diff", strings.NewReader(`---
				version: 1
				applications:
				  - name: app1
			`))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Content-type", "application/x-yaml")
			})

			It("returns a 404", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				expectNotFoundError("Space not found")
			})
		})
	})
})
