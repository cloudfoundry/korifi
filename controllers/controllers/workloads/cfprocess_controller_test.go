package workloads_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFProcessReconciler Integration Tests", func() {
	var (
		cfSpace   *korifiv1alpha1.CFSpace
		cfProcess *korifiv1alpha1.CFProcess
		cfApp     *korifiv1alpha1.CFApp
		cfBuild   *korifiv1alpha1.CFBuild
		cfRoute   *korifiv1alpha1.CFRoute
	)

	BeforeEach(func() {
		cfSpace = createSpace(testOrg)

		appGUID := GenerateGUID()
		buildGUID := GenerateGUID()
		packageGUID := GenerateGUID()

		// Technically the app controller should be creating this process based
		// on CFApp and CFBuild, but we want to drive testing with a specific
		// CFProcess instead of cascading (non-object-ref) state through other
		// resources. The app controller is only creating processes if they do
		// not exist, so ours is going to be "adopted"
		cfProcess = BuildCFProcessCRObject(GenerateGUID(), cfSpace.Status.GUID, appGUID, korifiv1alpha1.ProcessTypeWeb, "process command", "detected-command")
		Expect(adminClient.Create(ctx, cfProcess)).To(Succeed())

		cfApp = BuildCFAppCRObject(appGUID, cfSpace.Status.GUID)
		cfApp.Spec.EnvSecretName = cfApp.Name + "-env"
		cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: buildGUID}
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())

		cfPackage := BuildCFPackageCRObject(packageGUID, cfSpace.Status.GUID, cfApp.Name, "ref")
		Expect(adminClient.Create(ctx, cfPackage)).To(Succeed())

		cfBuild = BuildCFBuildObject(buildGUID, cfSpace.Status.GUID, packageGUID, cfApp.Name)

		appEnvSecret := BuildCFAppEnvVarsSecret(cfApp.Name, cfSpace.Status.GUID, map[string]string{
			"env-key": "env-val",
		})
		Expect(adminClient.Create(ctx, appEnvSecret)).To(Succeed())

		buildDropletStatus := BuildCFBuildDropletStatusObject(map[string]string{"web": "command-from-droplet"})
		cfBuild = createBuildWithDroplet(ctx, adminClient, cfBuild, buildDropletStatus)

		cfDomain := &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      GenerateGUID(),
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: "a" + uuid.NewString() + ".com",
			},
		}
		Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())

		destination := korifiv1alpha1.Destination{
			GUID:        GenerateGUID(),
			AppRef:      corev1.LocalObjectReference{Name: cfApp.Name},
			ProcessType: korifiv1alpha1.ProcessTypeWeb,
			Port:        tools.PtrTo(8080),
			Protocol:    tools.PtrTo("http1"),
		}
		cfRoute = &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfDomain.Name,
				Namespace: cfSpace.Status.GUID,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Host:     PrefixedGUID("test-host"),
				Path:     "",
				Protocol: "http",
				DomainRef: corev1.ObjectReference{
					Name:      cfDomain.Name,
					Namespace: cfSpace.Status.GUID,
				},
				Destinations: []korifiv1alpha1.Destination{destination},
			},
		}
		Expect(adminClient.Create(ctx, cfRoute)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfRoute, func() {
			cfRoute.Status = korifiv1alpha1.CFRouteStatus{
				Destinations:  []korifiv1alpha1.Destination{destination},
				CurrentStatus: korifiv1alpha1.ValidStatus,
				Description:   "ok",
			}
		})).To(Succeed())
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
			var createdCFProcess korifiv1alpha1.CFProcess
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: cfProcess.Name, Namespace: cfSpace.Status.GUID}, &createdCFProcess)).To(Succeed())

			g.Expect(createdCFProcess.Status.ObservedGeneration).To(Equal(createdCFProcess.Generation))
		}).Should(Succeed())
	})

	When("the CFApp desired state is STARTED", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		It("reconciles the CFProcess into an AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
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
			})
		})

		When("the app workload instances is set", func() {
			BeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
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
				Expect(k8s.PatchResource(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.Command = ""
				})).To(Succeed())
			})

			It("eventually creates an app workload using the command from the build", func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", "command-from-droplet"))
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
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.StartupProbe).To(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe).To(BeNil())
				})
			})

			It("does not set port related environment variables on the app workload", func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.Env).NotTo(ContainElements(
						MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_APP_PORT")}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("PORT")}),
					))
				})
			})
		})

		When("a CFApp desired state is updated to STOPPED", func() {
			BeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				})).To(Succeed())
			})

			It("deletes the AppWorkloads", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(ctx, &appWorkloads, client.InNamespace(cfSpace.Status.GUID), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: cfProcess.Name,
					})).To(Succeed())
					g.Expect(appWorkloads.Items).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the app process instances are scaled down to 0", func() {
			BeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = tools.PtrTo(0)
				})).To(Succeed())
			})

			It("deletes the app workload", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(context.Background(), &appWorkloads, client.InNamespace(cfProcess.Namespace), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: cfProcess.Name,
					})).To(Succeed())
					g.Expect(appWorkloads.Items).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the app process instances are unset", func() {
			BeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = nil
				})).To(Succeed())
			})

			It("does not delete the app workload", func() {
				Consistently(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(context.Background(), &appWorkloads, client.InNamespace(cfProcess.Namespace), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: cfProcess.Name,
					})).To(Succeed())
					g.Expect(appWorkloads.Items).NotTo(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("both the app-rev and the last-stop-app-rev are bumped", func() {
			var prevAppWorkloadName string

			BeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					prevAppWorkloadName = appWorkload.Name
				})

				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey] = "6"
					cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = "6"
				})).To(Succeed())
			})

			It("deletes the app workload and createas a new one", func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Name).NotTo(Equal(prevAppWorkloadName))
				})
			})
		})

		When("the app-rev is bumped and the last-stop-app-rev is not", func() {
			var appWorkload *korifiv1alpha1.AppWorkload

			BeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, workload korifiv1alpha1.AppWorkload) {
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

	When("the CFProcess has an http health check", func() {
		const (
			healthCheckEndpoint                 = "/healthy"
			healthCheckTimeoutSeconds           = 9
			healthCheckInvocationTimeoutSeconds = 3
		)

		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
					Type: "http",
					Data: korifiv1alpha1.HealthCheckData{
						HTTPEndpoint:             healthCheckEndpoint,
						InvocationTimeoutSeconds: healthCheckInvocationTimeoutSeconds,
						TimeoutSeconds:           healthCheckTimeoutSeconds,
					},
				}
			})).To(Succeed())
		})

		It("sets the startup and liveness probes correctly on the AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
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
		const (
			healthCheckTimeoutSeconds           = 9
			healthCheckInvocationTimeoutSeconds = 3
		)

		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
					Type: "port",
					Data: korifiv1alpha1.HealthCheckData{
						InvocationTimeoutSeconds: healthCheckInvocationTimeoutSeconds,
						TimeoutSeconds:           healthCheckTimeoutSeconds,
					},
				}
			})).To(Succeed())
		})

		It("sets the startup and liveness probes correctly on the AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
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
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{Type: "process"}
			})).To(Succeed())
		})

		It("does not set liveness and startup probes on the AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(cfProcess.Name, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				g.Expect(appWorkload.Spec.StartupProbe).To(BeNil())
				g.Expect(appWorkload.Spec.LivenessProbe).To(BeNil())
			})
		})
	})
})

func eventuallyCreatedAppWorkloadShould(processGUID, namespace string, shouldFn func(Gomega, korifiv1alpha1.AppWorkload)) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		var appWorkloads korifiv1alpha1.AppWorkloadList
		err := adminClient.List(context.Background(), &appWorkloads, client.InNamespace(namespace), client.MatchingLabels{
			korifiv1alpha1.CFProcessGUIDLabelKey: processGUID,
		})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(appWorkloads.Items).To(HaveLen(1))

		shouldFn(g, appWorkloads.Items[0])
	}).Should(Succeed())
}
