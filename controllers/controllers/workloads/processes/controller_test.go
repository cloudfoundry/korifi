package processes_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFProcessReconciler Integration Tests", func() {
	var (
		cfProcess *korifiv1alpha1.CFProcess
		cfApp     *korifiv1alpha1.CFApp
		cfBuild   *korifiv1alpha1.CFBuild
		cfRoute   *korifiv1alpha1.CFRoute
	)

	BeforeEach(func() {
		appEnvSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			StringData: map[string]string{
				"env-key": "env-val",
			},
		}
		Expect(adminClient.Create(ctx, appEnvSecret)).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
				Annotations: map[string]string{
					korifiv1alpha1.CFAppRevisionKey:         "5",
					korifiv1alpha1.CFAppLastStopRevisionKey: "2",
				},
				Finalizers: []string{
					korifiv1alpha1.CFAppFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "test-app-name",
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
				EnvSecretName: appEnvSecret.Name,
			},
		}
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())

		vcapApplicationSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			StringData: map[string]string{
				"VCAP_APPLICATION": "{}",
			},
		}
		Expect(adminClient.Create(ctx, vcapApplicationSecret)).To(Succeed())

		vcapServicesSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			StringData: map[string]string{
				"VCAP_SERVICES": "{}",
			},
		}
		Expect(adminClient.Create(ctx, vcapServicesSecret)).To(Succeed())

		Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
			cfApp.Status.VCAPApplicationSecretName = vcapApplicationSecret.Name
			cfApp.Status.VCAPServicesSecretName = vcapServicesSecret.Name
		})).To(Succeed())

		cfRoute = &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Destinations: []korifiv1alpha1.Destination{{
					GUID:        uuid.NewString(),
					AppRef:      corev1.LocalObjectReference{Name: cfApp.Name},
					ProcessType: korifiv1alpha1.ProcessTypeWeb,
					Port:        tools.PtrTo[int32](8080),
					Protocol:    tools.PtrTo("http1"),
				}},
			},
		}
		Expect(adminClient.Create(ctx, cfRoute)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfRoute, func() {
			cfRoute.Status = korifiv1alpha1.CFRouteStatus{
				Destinations: cfRoute.Spec.Destinations,
			}
		})).To(Succeed())

		cfBuild = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
		Expect(adminClient.Create(ctx, cfBuild)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfBuild, func() {
			cfBuild.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
				Registry: korifiv1alpha1.Registry{
					Image:            "image/registry/url",
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "some-image-pull-secret"}},
				},
			}
		})).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
			cfApp.Spec.CurrentDropletRef.Name = cfBuild.Name
		})).To(Succeed())

		cfProcessGUID := uuid.NewString()
		cfProcess = &korifiv1alpha1.CFProcess{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfProcessGUID,
				Namespace: testNamespace,
				Labels: map[string]string{
					korifiv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
					korifiv1alpha1.CFProcessGUIDLabelKey: cfProcessGUID,
					korifiv1alpha1.CFProcessTypeLabelKey: korifiv1alpha1.ProcessTypeWeb,
				},
			},
			Spec: korifiv1alpha1.CFProcessSpec{
				AppRef:           corev1.LocalObjectReference{Name: cfApp.Name},
				ProcessType:      korifiv1alpha1.ProcessTypeWeb,
				Command:          "process command",
				DetectedCommand:  "detected-command",
				DesiredInstances: tools.PtrTo[int32](1),
				MemoryMB:         1024,
				DiskQuotaMB:      100,
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfProcess)).To(Succeed())
	})

	It("sets owner references on CFProcess", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfProcess), cfProcess)).To(Succeed())
			g.Expect(cfProcess.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
				Kind:               "CFApp",
				Name:               cfApp.Name,
				UID:                cfApp.UID,
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
		}).Should(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfProcess), cfProcess)).To(Succeed())
			g.Expect(cfProcess.Status.ObservedGeneration).To(Equal(cfProcess.Generation))
		}).Should(Succeed())
	})

	When("the CFApp desired state is STARTED", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		It("reconciles the CFProcess into an AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				g.Expect(appWorkload.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
					APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
					Kind:               "CFProcess",
					Name:               cfProcess.Name,
					UID:                cfProcess.UID,
					Controller:         tools.PtrTo(true),
					BlockOwnerDeletion: tools.PtrTo(true),
				}))

				g.Expect(appWorkload.ObjectMeta.Labels).To(MatchAllKeys(Keys{
					korifiv1alpha1.CFAppGUIDLabelKey:     Equal(cfApp.Name),
					korifiv1alpha1.CFAppRevisionKey:      Equal(cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey]),
					korifiv1alpha1.CFProcessGUIDLabelKey: Equal(cfProcess.Name),
					korifiv1alpha1.CFProcessTypeLabelKey: Equal(cfProcess.Spec.ProcessType),
				}))

				g.Expect(appWorkload.ObjectMeta.Annotations).To(HaveKeyWithValue(
					korifiv1alpha1.CFAppLastStopRevisionKey, cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]),
				)

				g.Expect(appWorkload.Spec.GUID).To(Equal(cfProcess.Name))
				g.Expect(appWorkload.Spec.Version).To(Equal(cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey]))
				g.Expect(appWorkload.Spec.Image).To(Equal(cfBuild.Status.Droplet.Registry.Image))
				g.Expect(appWorkload.Spec.ImagePullSecrets).To(Equal(cfBuild.Status.Droplet.Registry.ImagePullSecrets))
				g.Expect(appWorkload.Spec.ProcessType).To(Equal(korifiv1alpha1.ProcessTypeWeb))
				g.Expect(appWorkload.Spec.AppGUID).To(Equal(cfApp.Name))
				g.Expect(appWorkload.Spec.Ports).To(ConsistOf(int32(8080)))
				g.Expect(appWorkload.Spec.Instances).To(Equal(int32(*cfProcess.Spec.DesiredInstances)))
				g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", "process command"))

				g.Expect(appWorkload.Spec.Resources.Limits.StorageEphemeral()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.DiskQuotaMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Limits.Memory()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.MemoryMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Requests.StorageEphemeral()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.DiskQuotaMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Requests.Memory()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.MemoryMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Requests.Cpu()).To(matchers.RepresentResourceQuantity(100, "m"))

				g.Expect(appWorkload.Spec.Env).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"Name": Equal("PORT")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("MEMORY_LIMIT")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_APPLICATION")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_APP_HOST")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_APP_PORT")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_SERVICES")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("env-key")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("CF_INSTANCE_PORTS")}),
				))

				g.Expect(appWorkload.Spec.RunnerName).To(Equal("cf-process-controller-test"))
			})
		})

		When("the CFProcess has an http health check", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
					Type: "http",
					Data: korifiv1alpha1.HealthCheckData{
						HTTPEndpoint:             "/healthy",
						InvocationTimeoutSeconds: 3,
						TimeoutSeconds:           9,
					},
				}
			})

			It("sets the startup and liveness probes on the AppWorkload", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.StartupProbe).ToNot(BeNil())
					g.Expect(appWorkload.Spec.StartupProbe.HTTPGet).ToNot(BeNil())
					g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Path).To(Equal("/healthy"))
					g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Port.IntValue()).To(Equal(8080))
					g.Expect(appWorkload.Spec.StartupProbe.InitialDelaySeconds).To(BeZero())
					g.Expect(appWorkload.Spec.StartupProbe.PeriodSeconds).To(BeEquivalentTo(2))
					g.Expect(appWorkload.Spec.StartupProbe.TimeoutSeconds).To(BeEquivalentTo(3))
					g.Expect(appWorkload.Spec.StartupProbe.FailureThreshold).To(BeEquivalentTo(5))

					g.Expect(appWorkload.Spec.LivenessProbe).ToNot(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe.InitialDelaySeconds).To(BeZero())
					g.Expect(appWorkload.Spec.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(30))
					g.Expect(appWorkload.Spec.LivenessProbe.TimeoutSeconds).To(BeEquivalentTo(3))
					g.Expect(appWorkload.Spec.LivenessProbe.FailureThreshold).To(BeEquivalentTo(1))
				})
			})
		})

		When("the CFProcess has a port health check", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
					Type: "port",
					Data: korifiv1alpha1.HealthCheckData{
						InvocationTimeoutSeconds: 3,
						TimeoutSeconds:           9,
					},
				}
			})

			It("sets the startup and liveness probes on the AppWorkload", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.StartupProbe).ToNot(BeNil())
					g.Expect(appWorkload.Spec.StartupProbe.TCPSocket).ToNot(BeNil())
					g.Expect(appWorkload.Spec.StartupProbe.TCPSocket.Port.IntValue()).To(Equal(8080))
					g.Expect(appWorkload.Spec.StartupProbe.InitialDelaySeconds).To(BeZero())
					g.Expect(appWorkload.Spec.StartupProbe.PeriodSeconds).To(BeEquivalentTo(2))
					g.Expect(appWorkload.Spec.StartupProbe.TimeoutSeconds).To(BeEquivalentTo(3))
					g.Expect(appWorkload.Spec.StartupProbe.FailureThreshold).To(BeEquivalentTo(5))

					g.Expect(appWorkload.Spec.LivenessProbe).ToNot(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe.InitialDelaySeconds).To(BeZero())
					g.Expect(appWorkload.Spec.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(30))
					g.Expect(appWorkload.Spec.LivenessProbe.TimeoutSeconds).To(BeEquivalentTo(3))
					g.Expect(appWorkload.Spec.LivenessProbe.FailureThreshold).To(BeEquivalentTo(1))
				})
			})
		})

		When("the CFProcess has a process health check", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{Type: "process"}
			})

			It("does not set liveness and startup probes on the AppWorkload", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.StartupProbe).To(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe).To(BeNil())
				})
			})
		})

		When("the app workload instances is set", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(k8s.Patch(ctx, adminClient, &appWorkload, func() {
						appWorkload.Status.ActualInstances = 3
					})).To(Succeed())
				})
			})

			It("updates the actual process instances", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfProcess), cfProcess)).To(Succeed())
					g.Expect(cfProcess.Status.ActualInstances).To(BeEquivalentTo(3))
				}).Should(Succeed())
			})
		})

		When("The process command field isn't set", func() {
			BeforeEach(func() {
				cfProcess.Spec.Command = ""
			})

			It("creates an app workload using the detected command", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", "detected-command"))
				})
			})
		})

		When("there are no route destinations for the process app", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, cfRoute, func() {
					cfRoute.Status.Destinations = []korifiv1alpha1.Destination{}
				})).To(Succeed())
			})

			It("does not create startup and liveness probes on the app workload", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.StartupProbe).To(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe).To(BeNil())
				})
			})

			It("does not set port related environment variables on the app workload", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.Env).NotTo(ContainElements(
						MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_APP_PORT")}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("PORT")}),
					))
				})
			})
		})

		When("a CFApp desired state is updated to STOPPED", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				})).To(Succeed())
			})

			It("deletes the AppWorkloads", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace))).To(Succeed())
					g.Expect(appWorkloads.Items).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the app process instances are scaled down to 0", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = tools.PtrTo[int32](0)
				})).To(Succeed())
			})

			It("deletes the app workload", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace))).To(Succeed())
					g.Expect(appWorkloads.Items).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the process desired instances are unset", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = nil
				})).To(Succeed())
			})

			It("does not delete the app workload", func() {
				Consistently(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace))).To(Succeed())
					g.Expect(appWorkloads.Items).NotTo(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("both the app-rev and the last-stop-app-rev are bumped", func() {
			var prevAppWorkloadName string

			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					prevAppWorkloadName = appWorkload.Name
				})

				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey] = "6"
					cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = "6"
				})).To(Succeed())
			})

			It("deletes the app workload and createas a new one", func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Name).NotTo(Equal(prevAppWorkloadName))
				})
			})
		})

		When("the app-rev is bumped and the last-stop-app-rev is not", func() {
			var appWorkload *korifiv1alpha1.AppWorkload

			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(func(g Gomega, workload korifiv1alpha1.AppWorkload) {
					appWorkload = &workload
				})

				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey] = "6"
				})).To(Succeed())
			})

			It("does not delete the app workload", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
				}, "1s").Should(Succeed())
			})
		})
	})
})

func eventuallyCreatedAppWorkloadShould(shouldFn func(Gomega, korifiv1alpha1.AppWorkload)) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		var appWorkloads korifiv1alpha1.AppWorkloadList
		g.Expect(adminClient.List(context.Background(), &appWorkloads, client.InNamespace(testNamespace))).To(Succeed())
		g.Expect(appWorkloads.Items).To(HaveLen(1))

		shouldFn(g, appWorkloads.Items[0])
	}).Should(Succeed())
}
