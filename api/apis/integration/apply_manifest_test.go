package integration_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

var _ = Describe("POST /v3/spaces/<space-guid>/actions/apply_manifest endpoint", func() {
	BeforeEach(func() {
		appRepo := new(repositories.AppRepo)
		processRepo := new(repositories.ProcessRepo)
		apiHandler := NewSpaceManifestHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			actions.NewApplyManifest(appRepo, processRepo).Invoke,
			repositories.NewOrgRepo("cf", k8sClient, 1*time.Minute),
			repositories.BuildCRClient,
			k8sConfig,
		)
		apiHandler.RegisterRoutes(router)
	})

	When("on the happy path", func() {
		var (
			namespace      *corev1.Namespace
			resp           *http.Response
			requestEnvVars map[string]string
			requestBody    string
		)

		const (
			appName = "app1"
			key1    = "KEY1"
			key2    = "KEY2"
		)

		BeforeEach(func() {
			namespaceGUID := generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
			Expect(
				k8sClient.Create(context.Background(), namespace),
			).To(Succeed())

			requestEnvVars = map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}
		})

		AfterEach(func() {
			k8sClient.Delete(context.Background(), namespace)
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequest(
				"POST",
				serverURI("/v3/spaces/", namespace.Name, "/actions/apply_manifest"),
				strings.NewReader(requestBody),
			)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/x-yaml")
			resp, err = new(http.Client).Do(req)
			Expect(err).NotTo(HaveOccurred())
		})

		When("no app with that name exists", func() {
			BeforeEach(func() {
				requestBody = fmt.Sprintf(`---
                  version: 1
                  applications:
                  - name: %s
                    env:
                      %s: %s
                      %s: %s
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
                      timeout: 90`, appName, key1, requestEnvVars[key1], key2, requestEnvVars[key2])
			})

			It("creates the applications in the manifest, the env var secret, and the processes, returns 202 and a job URI", func() {
				Expect(resp.StatusCode).To(Equal(202))

				body, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(BeEmpty())

				Expect(resp.Header.Get("Location")).To(Equal(serverURI("/v3/jobs/sync-space.apply_manifest-", namespace.Name)))

				var appList v1alpha1.CFAppList
				Eventually(func() []v1alpha1.CFApp {
					Expect(
						k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
					).To(Succeed())
					return appList.Items
				}).Should(HaveLen(1))

				app1 := appList.Items[0]
				Expect(app1.Spec.Name).To(Equal(appName))
				Expect(app1.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
				Expect(app1.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
				Expect(app1.Spec.EnvSecretName).NotTo(BeEmpty())

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

				var processList v1alpha1.CFProcessList
				Eventually(func() []v1alpha1.CFProcess {
					Expect(
						k8sClient.List(context.Background(), &processList, client.InNamespace(namespace.Name)),
					).To(Succeed())
					return processList.Items
				}).Should(HaveLen(2))

				Expect(processList.Items).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Spec": Equal(v1alpha1.CFProcessSpec{
							AppRef:      corev1.LocalObjectReference{Name: app1.Name},
							ProcessType: "web",
							Command:     "start-web.sh",
							HealthCheck: v1alpha1.HealthCheck{
								Type: "http",
								Data: v1alpha1.HealthCheckData{
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
						"Spec": Equal(v1alpha1.CFProcessSpec{
							AppRef:      corev1.LocalObjectReference{Name: app1.Name},
							ProcessType: "worker",
							Command:     "start-worker.sh",
							HealthCheck: v1alpha1.HealthCheck{
								Type: "http",
								Data: v1alpha1.HealthCheckData{
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

		When("an app with that name already exists with env vars and processes", func() {
			const (
				appGUID = "my-app-guid"
				key0    = "KEY0"
			)

			var (
				originalEnvVars         map[string]string
				originalExistingProcess v1alpha1.CFProcess
			)

			BeforeEach(func() {
				requestBody = fmt.Sprintf(`---
                  version: 1
                  applications:
                  - name: %s
                    env:
                      %s: %s
                      %s: %s
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
                      timeout: 90`, appName, key1, requestEnvVars[key1], key2, requestEnvVars[key2])

				beforeCtx := context.Background()
				Expect(
					k8sClient.Create(beforeCtx, &v1alpha1.CFApp{
						ObjectMeta: metav1.ObjectMeta{Name: appGUID, Namespace: namespace.Name},
						Spec: v1alpha1.CFAppSpec{
							Name:          appName,
							EnvSecretName: appGUID + "-env",
							DesiredState:  v1alpha1.StoppedState,
							Lifecycle: v1alpha1.Lifecycle{
								Type: v1alpha1.BuildpackLifecycle,
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

				originalExistingProcess = v1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-process-guid",
						Namespace: namespace.Name,
					},
					Spec: v1alpha1.CFProcessSpec{
						AppRef:           corev1.LocalObjectReference{Name: appGUID},
						ProcessType:      "existing",
						Command:          "do-the-thing.sh",
						HealthCheck:      v1alpha1.HealthCheck{Type: "process"},
						DesiredInstances: 1,
						MemoryMB:         123,
						Ports:            []int32{},
					},
				}

				Expect(
					k8sClient.Create(beforeCtx, &originalExistingProcess),
				).To(Succeed())

				Expect(
					k8sClient.Create(beforeCtx, &v1alpha1.CFProcess{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "web-process-guid",
							Namespace: namespace.Name,
						},
						Spec: v1alpha1.CFProcessSpec{
							AppRef:           corev1.LocalObjectReference{Name: appGUID},
							ProcessType:      "web",
							Command:          "original-command.sh",
							HealthCheck:      v1alpha1.HealthCheck{Type: "process"},
							DesiredInstances: 42,
							MemoryMB:         42,
							DiskQuotaMB:      42,
							Ports:            []int32{},
						},
					}),
				).To(Succeed())
			})

			It("updates the app with the manifest changes and returns a 202", func() {
				Expect(resp.StatusCode).To(Equal(202))

				body, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(BeEmpty())

				Expect(resp.Header.Get("Location")).To(Equal(serverURI("/v3/jobs/sync-space.apply_manifest-", namespace.Name)))

				var appList v1alpha1.CFAppList
				Eventually(func() []v1alpha1.CFApp {
					Expect(
						k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
					).To(Succeed())
					return appList.Items
				}).Should(HaveLen(1))

				app1 := appList.Items[0]
				Expect(app1.Spec.Name).To(Equal(appName))
				Expect(app1.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
				Expect(app1.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
				Expect(app1.Spec.EnvSecretName).NotTo(BeEmpty())

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

				var processList v1alpha1.CFProcessList
				Eventually(func() []v1alpha1.CFProcess {
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
						"Spec": Equal(v1alpha1.CFProcessSpec{
							AppRef:      corev1.LocalObjectReference{Name: app1.Name},
							ProcessType: "web",
							Command:     "new-command",
							HealthCheck: v1alpha1.HealthCheck{
								Type: "process",
							},
							DesiredInstances: 42,
							MemoryMB:         256,
							DiskQuotaMB:      128,
							Ports:            []int32{},
						}),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Spec": Equal(v1alpha1.CFProcessSpec{
							AppRef:      corev1.LocalObjectReference{Name: app1.Name},
							ProcessType: "worker",
							Command:     "start-worker.sh",
							HealthCheck: v1alpha1.HealthCheck{
								Type: "http",
								Data: v1alpha1.HealthCheckData{
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
		})
	})
})
