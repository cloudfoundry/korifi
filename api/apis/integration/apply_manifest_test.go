package integration_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("POST /v3/spaces/<space-guid>/actions/apply_manifest endpoint", func() {
	const (
		domainName = "my-domain.fun"
	)

	BeforeEach(func() {
		appRepo := repositories.NewAppRepo(k8sClient, namespaceRetriever, clientFactory, nsPermissions)
		domainRepo := repositories.NewDomainRepo(rootNamespace, k8sClient, namespaceRetriever, clientFactory)
		processRepo := repositories.NewProcessRepo(k8sClient, namespaceRetriever, clientFactory, nsPermissions)
		routeRepo := repositories.NewRouteRepo(k8sClient, namespaceRetriever, clientFactory, nsPermissions)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewSpaceManifestHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			domainName,
			actions.NewApplyManifest(appRepo, domainRepo, processRepo, routeRepo).Invoke,
			repositories.NewOrgRepo("cf", k8sClient, clientFactory, nsPermissions, 1*time.Minute, true),
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)
	})

	When("on the happy path", func() {
		var (
			namespace      *hnsv1alpha2.SubnamespaceAnchor
			requestEnvVars map[string]string
			requestBody    string
			domainGUID     string
		)

		const (
			appName = "app1"
			key1    = "KEY1"
			key2    = "KEY2"
		)

		BeforeEach(func() {
			domainGUID = generateGUID()
			org := createOrgAnchorAndNamespace(ctx, rootNamespace, generateGUID())
			namespace = createSpaceAnchorAndNamespace(ctx, org.Name, "spacename-"+generateGUID())

			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)

			requestEnvVars = map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}

			domain := &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      domainGUID,
					Namespace: rootNamespace,
				},
				Spec: networkingv1alpha1.CFDomainSpec{Name: domainName},
			}

			Expect(
				k8sClient.Create(context.Background(), domain),
			).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(context.Background(), domain)
			})
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(
				ctx,
				"POST",
				serverURI("/v3/spaces/", namespace.Name, "/actions/apply_manifest"),
				strings.NewReader(requestBody),
			)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/x-yaml")
			router.ServeHTTP(rr, req)
		})

		When("no app with that name exists", func() {
			const (
				host        = "custom"
				path        = "/path"
				routeString = host + "." + domainName + path
			)

			BeforeEach(func() {
				requestBody = fmt.Sprintf(`---
                  version: 1
                  applications:
                  - name: %s
                    env:
                      %s: %s
                      %s: %s
                    routes:
                    - route: %s
                    processes:
                    - type: web
                      command: start-web.sh
                      disk_quota: 512M
                      instances: 3
                      memory: 500M
                      health-check-type: http
                      health-check-http-endpoint: /stuff
                      health-check-invocation-timeout: 60
                      timeout: 60
                    - type: worker
                      command: start-worker.sh
                      disk_quota: 1G
                      memory: 256M
                      health-check-type: http
                      health-check-http-endpoint: /things
                      health-check-invocation-timeout: 90
                      timeout: 90`, appName, key1, requestEnvVars[key1], key2, requestEnvVars[key2], routeString)
			})

			It("creates the applications in the manifest, the env var secret, the processes, and the route, then returns 202 and a job URI", func() {
				Expect(rr.Code).To(Equal(202))

				body, err := ioutil.ReadAll(rr.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(BeEmpty())

				Expect(rr.Header().Get("Location")).To(Equal(serverURI("/v3/jobs/space.apply_manifest-", namespace.Name)))

				var app1 workloadsv1alpha1.CFApp
				By("confirming that the app was created", func() {
					var appList workloadsv1alpha1.CFAppList
					Eventually(func() []workloadsv1alpha1.CFApp {
						Expect(
							k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return appList.Items
					}).Should(HaveLen(1))

					app1 = appList.Items[0]
					Expect(app1.Spec.Name).To(Equal(appName))
					Expect(app1.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
					Expect(app1.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
					Expect(app1.Spec.EnvSecretName).NotTo(BeEmpty())
				})

				By("confirming that the secret was created", func() {
					secretNSName := types.NamespacedName{
						Name:      app1.Spec.EnvSecretName,
						Namespace: namespace.Name,
					}
					var secretRecord corev1.Secret
					Eventually(func() error {
						return k8sClient.Get(context.Background(), secretNSName, &secretRecord)
					}).Should(Succeed())
					Expect(secretRecord.Data).To(HaveLen(len(requestEnvVars)))

					for k, v := range requestEnvVars {
						Expect(secretRecord.Data).To(HaveKeyWithValue(k, BeEquivalentTo(v)))
					}
				})

				By("confirming that the processes from the manifest were created", func() {
					var processList workloadsv1alpha1.CFProcessList
					Eventually(func() []workloadsv1alpha1.CFProcess {
						Expect(
							k8sClient.List(context.Background(), &processList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return processList.Items
					}).Should(HaveLen(2))

					Expect(processList.Items).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Spec": Equal(workloadsv1alpha1.CFProcessSpec{
								AppRef:      corev1.LocalObjectReference{Name: app1.Name},
								ProcessType: "web",
								Command:     "start-web.sh",
								HealthCheck: workloadsv1alpha1.HealthCheck{
									Type: "http",
									Data: workloadsv1alpha1.HealthCheckData{
										HTTPEndpoint:             "/stuff",
										InvocationTimeoutSeconds: 60,
										TimeoutSeconds:           60,
									},
								},
								DesiredInstances: 3,
								MemoryMB:         500,
								DiskQuotaMB:      512,
								Ports:            []int32{},
							}),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Spec": Equal(workloadsv1alpha1.CFProcessSpec{
								AppRef:      corev1.LocalObjectReference{Name: app1.Name},
								ProcessType: "worker",
								Command:     "start-worker.sh",
								HealthCheck: workloadsv1alpha1.HealthCheck{
									Type: "http",
									Data: workloadsv1alpha1.HealthCheckData{
										HTTPEndpoint:             "/things",
										InvocationTimeoutSeconds: 90,
										TimeoutSeconds:           90,
									},
								},
								DesiredInstances: 0, // default value, since we didn't specify in manifest
								MemoryMB:         256,
								DiskQuotaMB:      1024,
								Ports:            []int32{},
							}),
						}),
					))
				})
			})

			When("the route doesn't exist", func() {
				It("creates the route from the manifest", func() {
					var app workloadsv1alpha1.CFApp
					var appList workloadsv1alpha1.CFAppList
					Eventually(func() []workloadsv1alpha1.CFApp {
						Expect(
							k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return appList.Items
					}).Should(HaveLen(1))

					app = appList.Items[0]

					var routeList networkingv1alpha1.CFRouteList
					Eventually(func() []networkingv1alpha1.CFRoute {
						Expect(
							k8sClient.List(context.Background(), &routeList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return routeList.Items
					}).Should(HaveLen(1))

					route := routeList.Items[0]
					Expect(route.Spec).To(MatchAllFields(Fields{
						"DomainRef": Equal(corev1.ObjectReference{Name: domainGUID, Namespace: rootNamespace}),
						"Host":      Equal(host),
						"Path":      Equal(path),
						"Protocol":  BeEquivalentTo("http"),
						"Destinations": ConsistOf(MatchAllFields(Fields{
							"GUID":        Not(BeEmpty()),
							"Port":        Equal(8080),
							"AppRef":      Equal(corev1.LocalObjectReference{Name: app.Name}),
							"ProcessType": Equal("web"),
							"Protocol":    Equal("http1"),
						})),
					}))
				})
			})

			When("a route exists, but the app isn't a destination", func() {
				const (
					routeGUID       = "my-route-guid"
					destinationGUID = "my-destination-guid"
					otherAppGUID    = "the-other-app-guid"
				)

				var originalRoute networkingv1alpha1.CFRoute

				BeforeEach(func() {
					originalRoute = networkingv1alpha1.CFRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      routeGUID,
							Namespace: namespace.Name,
						},
						Spec: networkingv1alpha1.CFRouteSpec{
							Host:     host,
							Path:     path,
							Protocol: "http",
							DomainRef: corev1.ObjectReference{
								Name:      domainGUID,
								Namespace: namespace.Name,
							},
							Destinations: []networkingv1alpha1.Destination{
								{
									GUID:        destinationGUID,
									Port:        8080,
									AppRef:      corev1.LocalObjectReference{Name: otherAppGUID},
									ProcessType: "web",
									Protocol:    "http1",
								},
							},
						},
					}
					Expect(
						k8sClient.Create(context.Background(), &originalRoute),
					).To(Succeed())
				})

				It("adds the app to the route's destinations", func() {
					var app workloadsv1alpha1.CFApp
					var appList workloadsv1alpha1.CFAppList
					Eventually(func() []workloadsv1alpha1.CFApp {
						Expect(
							k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return appList.Items
					}).Should(HaveLen(1))

					app = appList.Items[0]

					var routeList networkingv1alpha1.CFRouteList
					Eventually(func() []networkingv1alpha1.CFRoute {
						Expect(
							k8sClient.List(context.Background(), &routeList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return routeList.Items
					}).Should(HaveLen(1))

					route := routeList.Items[0]
					Expect(route.Spec).To(MatchAllFields(Fields{
						"DomainRef": Equal(corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: namespace.Name,
						}),
						"Host":     Equal(host),
						"Path":     Equal(path),
						"Protocol": BeEquivalentTo("http"),
						"Destinations": ConsistOf(
							Equal(originalRoute.Spec.Destinations[0]),
							MatchAllFields(Fields{
								"GUID":        Not(BeEmpty()),
								"Port":        Equal(8080),
								"AppRef":      Equal(corev1.LocalObjectReference{Name: app.Name}),
								"ProcessType": Equal("web"),
								"Protocol":    Equal("http1"),
							})),
					}))
				})
			})
		})

		When("an app with that name already exists with env vars, processes, and a route", func() {
			const (
				appGUID         = "my-app-guid"
				key0            = "KEY0"
				routeString     = "custom." + domainName + "/path"
				routeGUID       = "my-route-guid"
				destinationGUID = "my-destination-guid"
			)

			var (
				originalEnvVars         map[string]string
				originalExistingProcess workloadsv1alpha1.CFProcess
				originalRoute           networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				requestBody = fmt.Sprintf(`---
                  version: 1
                  applications:
                  - name: %s
                    env:
                      %s: %s
                      %s: %s
                    routes:
                    - route: %s
                    processes:
                    - type: web
                      command: new-command
                      disk_quota: 128M
                      memory: 256M
                    - type: worker
                      command: start-worker.sh
                      disk_quota: 1G
                      instances: 2
                      memory: 256M
                      health-check-type: http
                      health-check-http-endpoint: /things
                      health-check-invocation-timeout: 90
                      timeout: 90`, appName, key1, requestEnvVars[key1], key2, requestEnvVars[key2], routeString)

				beforeCtx := context.Background()
				Expect(
					k8sClient.Create(beforeCtx, &workloadsv1alpha1.CFApp{
						ObjectMeta: metav1.ObjectMeta{Name: appGUID, Namespace: namespace.Name},
						Spec: workloadsv1alpha1.CFAppSpec{
							Name:          appName,
							EnvSecretName: appGUID + "-env",
							DesiredState:  workloadsv1alpha1.StoppedState,
							Lifecycle: workloadsv1alpha1.Lifecycle{
								Type: workloadsv1alpha1.BuildpackLifecycle,
							},
						},
					}),
				).To(Succeed())

				originalEnvVars = map[string]string{
					key0: "VAL0",
					key1: "original-value", // This variable will change after the manifest is applied
				}
				Expect(
					k8sClient.Create(beforeCtx, &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      appGUID + "-env",
							Namespace: namespace.Name,
						},
						StringData: originalEnvVars,
					}),
				).To(Succeed())

				originalExistingProcess = workloadsv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-process-guid",
						Namespace: namespace.Name,
					},
					Spec: workloadsv1alpha1.CFProcessSpec{
						AppRef:           corev1.LocalObjectReference{Name: appGUID},
						ProcessType:      "existing",
						Command:          "do-the-thing.sh",
						HealthCheck:      workloadsv1alpha1.HealthCheck{Type: "process"},
						DesiredInstances: 1,
						MemoryMB:         123,
						Ports:            []int32{},
					},
				}

				Expect(
					k8sClient.Create(beforeCtx, &originalExistingProcess),
				).To(Succeed())

				Expect(
					k8sClient.Create(beforeCtx, &workloadsv1alpha1.CFProcess{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "web-process-guid",
							Namespace: namespace.Name,
						},
						Spec: workloadsv1alpha1.CFProcessSpec{
							AppRef:           corev1.LocalObjectReference{Name: appGUID},
							ProcessType:      "web",
							Command:          "original-command.sh",
							HealthCheck:      workloadsv1alpha1.HealthCheck{Type: "process"},
							DesiredInstances: 42,
							MemoryMB:         42,
							DiskQuotaMB:      42,
							Ports:            []int32{},
						},
					}),
				).To(Succeed())

				originalRoute = networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID,
						Namespace: namespace.Name,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "custom",
						Path:     "/path",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: namespace.Name,
						},
						Destinations: []networkingv1alpha1.Destination{
							{
								GUID:        destinationGUID,
								Port:        8080,
								AppRef:      corev1.LocalObjectReference{Name: appGUID},
								ProcessType: "web",
								Protocol:    "http1",
							},
						},
					},
				}
				Expect(
					k8sClient.Create(beforeCtx, &originalRoute),
				).To(Succeed())
			})

			It("updates the app with the manifest changes and returns a 202", func() {
				Expect(rr.Code).To(Equal(202))

				body, err := ioutil.ReadAll(rr.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(BeEmpty())

				Expect(rr.Header().Get("Location")).To(Equal(serverURI("/v3/jobs/space.apply_manifest-", namespace.Name)))

				var app1 workloadsv1alpha1.CFApp
				By("confirming that the app fields are unchanged", func() {
					var appList workloadsv1alpha1.CFAppList
					Eventually(func() []workloadsv1alpha1.CFApp {
						Expect(
							k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return appList.Items
					}).Should(HaveLen(1))

					app1 = appList.Items[0]
					Expect(app1.Spec.Name).To(Equal(appName))
					Expect(app1.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
					Expect(app1.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
					Expect(app1.Spec.EnvSecretName).NotTo(BeEmpty())
				})

				By("confirming that the new env vars were added to the secret", func() {
					secretNSName := types.NamespacedName{
						Name:      app1.Spec.EnvSecretName,
						Namespace: namespace.Name,
					}
					var updatedSecret corev1.Secret
					Eventually(func() map[string][]byte {
						Expect(
							k8sClient.Get(context.Background(), secretNSName, &updatedSecret),
						).To(Succeed())
						return updatedSecret.Data
					}, 3*time.Second).Should(MatchAllKeys(Keys{
						key0: BeEquivalentTo(originalEnvVars[key0]),
						key1: BeEquivalentTo(requestEnvVars[key1]),
						key2: BeEquivalentTo(requestEnvVars[key2]),
					}))
				})

				By("confirming that the new processes were created", func() {
					var processList workloadsv1alpha1.CFProcessList
					Eventually(func() []workloadsv1alpha1.CFProcess {
						Expect(
							k8sClient.List(context.Background(), &processList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return processList.Items
					}).Should(HaveLen(3))

					Expect(processList.Items).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Spec": Equal(originalExistingProcess.Spec),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Spec": Equal(workloadsv1alpha1.CFProcessSpec{
								AppRef:      corev1.LocalObjectReference{Name: app1.Name},
								ProcessType: "web",
								Command:     "new-command",
								HealthCheck: workloadsv1alpha1.HealthCheck{
									Type: "process",
								},
								DesiredInstances: 42,
								MemoryMB:         256,
								DiskQuotaMB:      128,
								Ports:            []int32{},
							}),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Spec": Equal(workloadsv1alpha1.CFProcessSpec{
								AppRef:      corev1.LocalObjectReference{Name: app1.Name},
								ProcessType: "worker",
								Command:     "start-worker.sh",
								HealthCheck: workloadsv1alpha1.HealthCheck{
									Type: "http",
									Data: workloadsv1alpha1.HealthCheckData{
										HTTPEndpoint:             "/things",
										InvocationTimeoutSeconds: 90,
										TimeoutSeconds:           90,
									},
								},
								DesiredInstances: 2,
								MemoryMB:         256,
								DiskQuotaMB:      1024,
								Ports:            []int32{},
							}),
						}),
					))
				})

				By("confirming that the route is unchanged and no new route was created", func() {
					var routeList networkingv1alpha1.CFRouteList
					Consistently(func() []networkingv1alpha1.CFRoute {
						Expect(
							k8sClient.List(context.Background(), &routeList, client.InNamespace(namespace.Name)),
						).To(Succeed())
						return routeList.Items
					}, "3s").Should(HaveLen(1))

					Expect(routeList.Items[0].Spec).To(Equal(originalRoute.Spec))
				})
			})
		})
	})
})
