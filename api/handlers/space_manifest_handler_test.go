package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SpaceManifestHandler", func() {
	const (
		appGUID           = "app-guid"
		spaceGUID         = "my-space"
		defaultDomainName = "default-domain.com"
		defaultDomainGUID = "default-domain-guid"
		rootNamespace     = "cf"
	)

	var (
		manifestRepo *fake.CFManifestRepository
		req          *http.Request
		requestBody  *strings.Reader
	)

	BeforeEach(func() {
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		manifestRepo = new(fake.CFManifestRepository)
		manifestRepo.GetDomainByNameReturns(repositories.DomainRecord{
			Name:      defaultDomainName,
			GUID:      defaultDomainGUID,
			Namespace: rootNamespace,
		}, nil)

		apiHandler := NewSpaceManifestHandler(
			*serverURL,
			defaultDomainName,
			decoderValidator,
			manifestRepo,
		)
		apiHandler.RegisterRoutes(router)
	})

	Describe("POST /v3/spaces/{spaceGUID}/actions/apply_manifest", func() {
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

			manifestRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			manifestRepo.CreateAppReturns(repositories.AppRecord{GUID: appGUID}, nil)
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/spaces/"+spaceGUID+"/actions/apply_manifest", requestBody)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/x-yaml")

			router.ServeHTTP(rr, req)
		})

		It("returns 202 with a Location header", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))

			Expect(rr).To(HaveHTTPHeaderWithValue("Location", ContainSubstring("space.apply_manifest~"+spaceGUID)))
		})

		It("fetches the app by name and space using the authInfo from the context", func() {
			Expect(manifestRepo.GetAppByNameAndSpaceCallCount()).To(Equal(1))

			_, actualAuthInfo, actualAppName, actualSpaceGUID := manifestRepo.GetAppByNameAndSpaceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppName).To(Equal("app1"))
			Expect(actualSpaceGUID).To(Equal(spaceGUID))
		})

		When("fetching the app returns a Forbidden error", func() {
			BeforeEach(func() {
				manifestRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(errors.New("boom"), repositories.AppResourceType))
			})

			It("returns a NotFound error", func() {
				expectNotFoundError("App not found")
			})

			It("doesn't create an App", func() {
				Expect(manifestRepo.CreateAppCallCount()).To(Equal(0))
			})
		})

		When("fetching the app returns a non-API error", func() {
			BeforeEach(func() {
				manifestRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("doesn't create an App", func() {
				Expect(manifestRepo.CreateAppCallCount()).To(Equal(0))
			})
		})

		It("creates the app and its processes", func() {
			Expect(manifestRepo.CreateAppCallCount()).To(Equal(1))

			_, actualAuthInfo, appMessage := manifestRepo.CreateAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(appMessage.Name).To(Equal("app1"))
			Expect(appMessage.SpaceGUID).To(Equal(spaceGUID))
		})

		When("creating the app errors", func() {
			BeforeEach(func() {
				manifestRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		It("creates a web process with the given resource properties", func() {
			Expect(manifestRepo.CreateProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, processMessage := manifestRepo.CreateProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(processMessage.AppGUID).To(Equal(appGUID))
			Expect(processMessage.SpaceGUID).To(Equal(spaceGUID))
			Expect(processMessage.Type).To(Equal("web"))
			Expect(processMessage.MemoryMB).To(Equal(int64(256)))
		})

		When("creating a process errors", func() {
			BeforeEach(func() {
				manifestRepo.CreateProcessReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the manifest includes application resource properties, but no web process", func() {
			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                    version: 1
                    applications:
                    - name: app1
                      default-route: true
                      memory: 128M
                    `)
			})

			It("creates a web process with the given resource properties", func() {
				Expect(manifestRepo.CreateProcessCallCount()).To(Equal(1))
				_, _, processMessage := manifestRepo.CreateProcessArgsForCall(0)
				Expect(processMessage.AppGUID).To(Equal("app-guid"))
				Expect(processMessage.Type).To(Equal("web"))
				Expect(processMessage.MemoryMB).To(Equal(int64(128)))
			})
		})

		When("the app exists", func() {
			const appEtcdUID = types.UID("my-app-etcd-uid")

			BeforeEach(func() {
				requestBody = strings.NewReader(`---
                    version: 1
                    applications:
                    - name: app1
                      default-route: false
                      memory: 128M
                      env:
                        FOO: bar
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

				manifestRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{
					Name:      "app1",
					GUID:      appGUID,
					EtcdUID:   appEtcdUID,
					SpaceGUID: spaceGUID,
				}, nil)
			})

			It("updates the app env vars", func() {
				Expect(manifestRepo.CreateOrPatchAppEnvVarsCallCount()).To(Equal(1))

				_, actualAuthInfo, message := manifestRepo.CreateOrPatchAppEnvVarsArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(message.AppGUID).To(Equal(appGUID))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))
				Expect(message.AppEtcdUID).To(Equal(appEtcdUID))
				Expect(message.EnvironmentVariables).To(Equal(map[string]string{"FOO": "bar"}))
			})

			When("updating the env vars errors", func() {
				BeforeEach(func() {
					manifestRepo.CreateOrPatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{}, errors.New("boom"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})

			It("checks if the process exists correctly", func() {
				Expect(manifestRepo.GetProcessByAppTypeAndSpaceCallCount()).To(Equal(1))

				_, actualAuthInfo, actualAppGUID, actualProcessType, actualSpaceGUID := manifestRepo.GetProcessByAppTypeAndSpaceArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualAppGUID).To(Equal(appGUID))
				Expect(actualProcessType).To(Equal("web"))
				Expect(actualSpaceGUID).To(Equal(spaceGUID))
			})

			When("checking if the process exists errors", func() {
				BeforeEach(func() {
					manifestRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, errors.New("boom"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})

			When("the process already exists", func() {
				BeforeEach(func() {
					manifestRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{GUID: "totes-real"}, nil)
				})

				It("patches the process", func() {
					Expect(manifestRepo.PatchProcessCallCount()).To(Equal(1))

					_, actualAuthInfo, message := manifestRepo.PatchProcessArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
					Expect(message.ProcessGUID).To(Equal("totes-real"))
					Expect(message.SpaceGUID).To(Equal(spaceGUID))
				})

				When("patching the process errors", func() {
					BeforeEach(func() {
						manifestRepo.PatchProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
					})

					It("returns an error", func() {
						expectUnknownError()
					})
				})
			})

			When("the process doesn't exist", func() {
				BeforeEach(func() {
					manifestRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, apierrors.NewNotFoundError(nil, repositories.ProcessResourceType))
				})

				It("creates the process", func() {
					Expect(manifestRepo.CreateProcessCallCount()).To(Equal(1))

					_, actualAuthInfo, message := manifestRepo.CreateProcessArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
					Expect(message.AppGUID).To(Equal(appGUID))
					Expect(message.SpaceGUID).To(Equal(spaceGUID))
					Expect(message.Type).To(Equal("web"))
				})

				When("creating the process errors", func() {
					BeforeEach(func() {
						manifestRepo.CreateProcessReturns(errors.New("boom"))
					})

					It("returns an error", func() {
						expectUnknownError()
					})
				})
			})

			When("default route is specified for the app, and no routes are specified", func() {
				BeforeEach(func() {
					requestBody = strings.NewReader(`---
                        version: 1
                        applications:
                        - name: app1
                          default-route: true
                          memory: 128M
                          env:
                            FOO: bar
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

				It("checks for existing routes correctly", func() {
					Expect(manifestRepo.ListRoutesForAppCallCount()).To(Equal(1))

					_, actualAuthInfo, actualAppGUID, actualSpaceGUID := manifestRepo.ListRoutesForAppArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
					Expect(actualAppGUID).To(Equal(appGUID))
					Expect(actualSpaceGUID).To(Equal(spaceGUID))
				})

				When("checking for existing routes fails", func() {
					BeforeEach(func() {
						manifestRepo.ListRoutesForAppReturns(nil, errors.New("boom"))
					})

					It("returns the error", func() {
						expectUnknownError()
					})
				})

				When("the app has no existing route destinations", func() {
					It("fetches the default domain, and calls create route for the default destination", func() {
						Expect(manifestRepo.GetOrCreateRouteCallCount()).To(Equal(1))
						_, _, createMessage := manifestRepo.GetOrCreateRouteArgsForCall(0)
						Expect(createMessage.Host).To(Equal("app1"))
						Expect(createMessage.Path).To(Equal(""))
						Expect(createMessage.DomainGUID).To(Equal(defaultDomainGUID))

						Expect(manifestRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
						_, _, destinationsMessage := manifestRepo.AddDestinationsToRouteArgsForCall(0)
						Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
						Expect(destinationsMessage.NewDestinations[0].AppGUID).To(Equal(appGUID))
					})

					When("fetching the default domain fails with a NotFound error", func() {
						BeforeEach(func() {
							manifestRepo.GetDomainByNameReturns(repositories.DomainRecord{}, apierrors.NewNotFoundError(errors.New("boom"), repositories.DomainResourceType))
						})

						It("returns an UnprocessibleEntity error with a friendly message", func() {
							expectUnprocessableEntityError(fmt.Sprintf("The configured default domain %q was not found", defaultDomainName))
						})
					})

					When("fetching the default domain fails", func() {
						BeforeEach(func() {
							manifestRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("fail-on-purpose"))
						})

						It("returns an error", func() {
							expectUnknownError()
						})
					})
				})

				When("the app already has a route destination", func() {
					BeforeEach(func() {
						manifestRepo.ListRoutesForAppReturns([]repositories.RouteRecord{{
							GUID: "some-other-route-guid",
						}}, nil)
					})

					It("succeeds", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
					})

					It("does not call GetOrCreateRoute", func() {
						Expect(manifestRepo.GetOrCreateRouteCallCount()).To(Equal(0))
						Expect(manifestRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
					})
				})

				When("a route is specified for the app", func() {
					BeforeEach(func() {
						manifestRepo.GetDomainByNameReturns(repositories.DomainRecord{
							Name:      "my-domain.com",
							GUID:      "my-domain-guid",
							Namespace: "my-domain-ns",
						}, nil)
						manifestRepo.GetOrCreateRouteReturns(repositories.RouteRecord{
							GUID:         "route-guid",
							SpaceGUID:    spaceGUID,
							Destinations: []repositories.DestinationRecord{{GUID: "existing-destination-guid"}},
						}, nil)

						requestBody = strings.NewReader(`---
                            version: 1
                            applications:
                            - name: app1
                              memory: 128M
                              routes:
                              - route: my-app.my-domain.com/path
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

					It("creates or updates the route", func() {
						Expect(manifestRepo.GetDomainByNameCallCount()).To(Equal(1))
						_, actualAuthInfo, domainName := manifestRepo.GetDomainByNameArgsForCall(0)
						Expect(actualAuthInfo).To(Equal(authInfo))
						Expect(domainName).To(Equal("my-domain.com"))

						Expect(manifestRepo.GetOrCreateRouteCallCount()).To(Equal(1))
						_, actualAuthInfo, getOrCreateMessage := manifestRepo.GetOrCreateRouteArgsForCall(0)
						Expect(actualAuthInfo).To(Equal(authInfo))
						Expect(getOrCreateMessage.Host).To(Equal("my-app"))
						Expect(getOrCreateMessage.Path).To(Equal("/path"))
						Expect(getOrCreateMessage.SpaceGUID).To(Equal(spaceGUID))
						Expect(getOrCreateMessage.DomainGUID).To(Equal("my-domain-guid"))
						Expect(getOrCreateMessage.DomainName).To(Equal("my-domain.com"))
						Expect(getOrCreateMessage.DomainNamespace).To(Equal("my-domain-ns"))

						Expect(manifestRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
						_, actualAuthInfo, addDestMessage := manifestRepo.AddDestinationsToRouteArgsForCall(0)
						Expect(actualAuthInfo).To(Equal(authInfo))
						Expect(addDestMessage.RouteGUID).To(Equal("route-guid"))
						Expect(addDestMessage.SpaceGUID).To(Equal(spaceGUID))
						Expect(addDestMessage.ExistingDestinations).To(ConsistOf(repositories.DestinationRecord{GUID: "existing-destination-guid"}))
						Expect(addDestMessage.NewDestinations).To(ConsistOf(repositories.DestinationMessage{
							AppGUID:     appGUID,
							ProcessType: "web",
							Port:        8080,
							Protocol:    "http1",
						}))
					})

					When("fetching the domain errors", func() {
						BeforeEach(func() {
							manifestRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("boom"))
						})

						It("doesn't create the route", func() {
							Expect(manifestRepo.GetOrCreateRouteCallCount()).To(Equal(0))
						})

						It("doesn't add destinations to a route", func() {
							Expect(manifestRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
						})

						It("errors", func() {
							expectUnknownError()
						})
					})

					When("fetching/creating the route errors", func() {
						BeforeEach(func() {
							manifestRepo.GetOrCreateRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
						})

						It("doesn't add destinations to a route", func() {
							Expect(manifestRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
						})

						It("errors", func() {
							expectUnknownError()
						})
					})

					When("adding the destination to the route errors", func() {
						BeforeEach(func() {
							manifestRepo.AddDestinationsToRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
						})

						It("errors", func() {
							expectUnknownError()
						})
					})

					When("defaultRoute:true is set on the manifest along with the routes", func() {
						BeforeEach(func() {
							requestBody = strings.NewReader(`---
                                version: 1
                                applications:
                                - name: app1
                                  default-route: true
                                  memory: 128M
                                  routes:
                                  - route: NOT-MY-APP.my-domain.com/path
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

						It("is ignored, and AddDestinationsToRoute is called without adding a default destination to the existing route list", func() {
							Expect(manifestRepo.GetOrCreateRouteCallCount()).To(Equal(1))
							_, _, createMessage := manifestRepo.GetOrCreateRouteArgsForCall(0)
							Expect(createMessage.Host).To(Equal("NOT-MY-APP"))
							Expect(createMessage.Path).To(Equal("/path"))
							Expect(createMessage.DomainGUID).To(Equal("my-domain-guid"))

							Expect(manifestRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
							_, _, destinationsMessage := manifestRepo.AddDestinationsToRouteArgsForCall(0)
							Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
						})
					})
				})
			})
		})

		Describe("Invalid manifest validation", func() {
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

				It("still returns 202", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
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

				It("does not talk to the manifest repository", func() {
					Expect(manifestRepo.Invocations()).To(BeEmpty())
				})
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
				manifestRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("foo"))
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
				manifestRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(errors.New("foo"), repositories.SpaceResourceType))
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
