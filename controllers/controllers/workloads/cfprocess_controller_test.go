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
		testProcessGUID string
		testAppGUID     string
		testBuildGUID   string
		testPackageGUID string
		cfSpace         *korifiv1alpha1.CFSpace
		cfProcess       *korifiv1alpha1.CFProcess
		cfPackage       *korifiv1alpha1.CFPackage
		cfApp           *korifiv1alpha1.CFApp
		cfBuild         *korifiv1alpha1.CFBuild
	)

	const (
		CFAppGUIDLabelKey     = "korifi.cloudfoundry.org/app-guid"
		cfAppRevisionKey      = "korifi.cloudfoundry.org/app-rev"
		CFProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
		CFProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"

		processTypeWeb           = "web"
		processTypeWebCommand    = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
		detectedCommand          = "detected-command"
		processTypeWorker        = "worker"
		processTypeWorkerCommand = "bundle exec rackup config.ru"
		port8080                 = 8080
		port9000                 = 9000
	)

	BeforeEach(func() {
		cfSpace = createSpace(cfOrg)
		testAppGUID = GenerateGUID()
		testProcessGUID = GenerateGUID()
		testBuildGUID = GenerateGUID()
		testPackageGUID = GenerateGUID()

		// Technically the app controller should be creating this process based on CFApp and CFBuild, but we
		// want to drive testing with a specific CFProcess instead of cascading (non-object-ref) state through
		// other resources.
		cfProcess = BuildCFProcessCRObject(testProcessGUID, cfSpace.Status.GUID, testAppGUID, processTypeWeb, processTypeWebCommand, detectedCommand)
		cfApp = BuildCFAppCRObject(testAppGUID, cfSpace.Status.GUID)
		cfApp.Spec.EnvSecretName = testAppGUID + "-env"
		cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: testBuildGUID}

		cfPackage = BuildCFPackageCRObject(testPackageGUID, cfSpace.Status.GUID, testAppGUID, "ref")
		cfBuild = BuildCFBuildObject(testBuildGUID, cfSpace.Status.GUID, testPackageGUID, testAppGUID)

		appEnvSecret := BuildCFAppEnvVarsSecret(testAppGUID, cfSpace.Status.GUID, map[string]string{
			"b-test-env-key-second": "b-test-env-val-second",
			"c-test-env-key-third":  "c-test-env-val-third",
			"a-test-env-key-first":  "a-test-env-val-first",
		})
		Expect(adminClient.Create(ctx, appEnvSecret)).To(Succeed())

		Expect(adminClient.Create(ctx, cfPackage)).To(Succeed())
		buildDropletStatus := BuildCFBuildDropletStatusObject(map[string]string{"web": "command-from-droplet"}, []int32{})
		cfBuild = createBuildWithDroplet(ctx, adminClient, cfBuild, buildDropletStatus)

		Expect(adminClient.Create(ctx, cfProcess)).To(Succeed())
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())
	})

	It("eventually reconciles to set owner references on CFProcess", func() {
		Eventually(func() []metav1.OwnerReference {
			var createdCFProcess korifiv1alpha1.CFProcess
			err := adminClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: cfSpace.Status.GUID}, &createdCFProcess)
			if err != nil {
				return nil
			}
			return createdCFProcess.GetOwnerReferences()
		}).Should(ConsistOf(metav1.OwnerReference{
			APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
			Kind:               "CFApp",
			Name:               cfApp.Name,
			UID:                cfApp.UID,
			Controller:         tools.PtrTo(true),
			BlockOwnerDeletion: tools.PtrTo(true),
		}))
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			var createdCFProcess korifiv1alpha1.CFProcess
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: cfSpace.Status.GUID}, &createdCFProcess)).To(Succeed())

			g.Expect(createdCFProcess.Status.ObservedGeneration).To(Equal(createdCFProcess.Generation))
		}).Should(Succeed())
	})

	When("the CFApp desired state is STARTED", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				var updatedCFApp korifiv1alpha1.CFApp
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), &updatedCFApp)).To(Succeed())

				g.Expect(appWorkload.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
					APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
					Kind:               "CFProcess",
					Name:               cfProcess.Name,
					UID:                cfProcess.UID,
					Controller:         tools.PtrTo(true),
					BlockOwnerDeletion: tools.PtrTo(true),
				}))
				g.Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFAppGUIDLabelKey, testAppGUID))
				g.Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppRevisionKey, cfApp.Annotations[cfAppRevisionKey]))
				g.Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessGUIDLabelKey, testProcessGUID))
				g.Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessTypeLabelKey, cfProcess.Spec.ProcessType))

				g.Expect(appWorkload.ObjectMeta.Annotations).To(HaveKeyWithValue(korifiv1alpha1.CFAppLastStopRevisionKey, cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]))

				g.Expect(appWorkload.Spec.GUID).To(Equal(cfProcess.Name))
				g.Expect(appWorkload.Spec.Version).To(Equal(cfApp.Annotations[cfAppRevisionKey]))
				g.Expect(appWorkload.Spec.Image).To(Equal(cfBuild.Status.Droplet.Registry.Image))
				g.Expect(appWorkload.Spec.ImagePullSecrets).To(Equal(cfBuild.Status.Droplet.Registry.ImagePullSecrets))
				g.Expect(appWorkload.Spec.ProcessType).To(Equal(processTypeWeb))
				g.Expect(appWorkload.Spec.AppGUID).To(Equal(cfApp.Name))
				g.Expect(appWorkload.Spec.Ports).To(Equal(cfProcess.Spec.Ports))
				g.Expect(appWorkload.Spec.Instances).To(Equal(int32(*cfProcess.Spec.DesiredInstances)))

				g.Expect(appWorkload.Spec.Resources.Limits.StorageEphemeral()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.DiskQuotaMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Limits.Memory()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.MemoryMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Requests.StorageEphemeral()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.DiskQuotaMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Requests.Memory()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.MemoryMB, "Mi"))
				g.Expect(appWorkload.Spec.Resources.Requests.Cpu()).To(matchers.RepresentResourceQuantity(100, "m"), "expected cpu request to be 100m (Based on 1024MiB memory)")

				// TODO: Write new test for env var ordering to better catch map iterator ordering instability
				// We expect this test will still continue to pass and failures will not be considered flakes, but may
				// exhibit false positives should the required sort code be removed.
				g.Expect(appWorkload.Spec.Env).To(HaveExactElements(
					Equal(corev1.EnvVar{Name: "PORT", Value: "8080"}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("VCAP_APPLICATION"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: updatedCFApp.Status.VCAPApplicationSecretName,
								},
								Key: "VCAP_APPLICATION",
							})),
						})),
					}),
					Equal(corev1.EnvVar{Name: "VCAP_APP_HOST", Value: "0.0.0.0"}),
					Equal(corev1.EnvVar{Name: "VCAP_APP_PORT", Value: "8080"}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("VCAP_SERVICES"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: updatedCFApp.Status.VCAPServicesSecretName,
								},
								Key: "VCAP_SERVICES",
							})),
						})),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("a-test-env-key-first"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "a-test-env-key-first",
							})),
						})),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("b-test-env-key-second"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "b-test-env-key-second",
							})),
						})),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("c-test-env-key-third"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "c-test-env-key-third",
							})),
						})),
					}),
				))
				g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", processTypeWebCommand))
			})
		})

		When("The process command field isn't set", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.Command = ""
				})).To(Succeed())
			})

			It("eventually creates an app workload using the command from the build", func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", "command-from-droplet"))
				})
			})
		})

		When("a CFApp desired state is updated to STOPPED", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				})).To(Succeed())
			})

			It("eventually deletes the AppWorkloads", func() {
				Eventually(func() ([]korifiv1alpha1.AppWorkload, error) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := adminClient.List(ctx, &appWorkloads, client.InNamespace(cfSpace.Status.GUID), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
					})
					return appWorkloads.Items, err
				}).Should(BeEmpty(), "Timed out waiting for deletion of AppWorkload/%s in namespace %s to cause NotFound error", testProcessGUID, cfSpace.Status.GUID)
			})
		})

		When("the app process instances are scaled down to 0", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = tools.PtrTo(0)
				})).To(Succeed())
			})

			It("deletes the app workload", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := adminClient.List(context.Background(), &appWorkloads, client.InNamespace(cfProcess.Namespace), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
					})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(appWorkloads.Items).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the app process instances are unset", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = nil
				})).To(Succeed())
			})

			It("does not delete the app workload", func() {
				Consistently(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := adminClient.List(context.Background(), &appWorkloads, client.InNamespace(cfProcess.Namespace), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
					})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(appWorkloads.Items).NotTo(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("both the app-rev and the last-stop-app-rev are bumped", func() {
			var prevAppWorkloadName string

			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(context.Background(), &appWorkloads,
						client.InNamespace(cfSpace.Status.GUID),
						client.MatchingLabels{
							korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
						}),
					).To(Succeed())
					g.Expect(appWorkloads.Items).To(HaveLen(1))
					prevAppWorkloadName = appWorkloads.Items[0].Name
				}).Should(Succeed())

				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey] = "6"
					cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = "6"
				})).To(Succeed())
			})

			It("deletes the app workload and createas a new one", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(context.Background(), &appWorkloads,
						client.InNamespace(cfSpace.Status.GUID),
						client.MatchingLabels{
							korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
						}),
					).To(Succeed())
					g.Expect(appWorkloads.Items).To(HaveLen(1))
					g.Expect(appWorkloads.Items[0].Name).ToNot(Equal(prevAppWorkloadName))
				}).Should(Succeed())
			})
		})

		When("the app-rev is bumped and the last-stop-app-rev is not", func() {
			var appWorkloadName string

			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					g.Expect(adminClient.List(context.Background(), &appWorkloads,
						client.InNamespace(cfSpace.Status.GUID),
						client.MatchingLabels{
							korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
						}),
					).To(Succeed())
					g.Expect(appWorkloads.Items).To(HaveLen(1))
					appWorkloadName = appWorkloads.Items[0].Name
				}).Should(Succeed())

				Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
					cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey] = "6"
				})).To(Succeed())
			})

			It("does not delete the app workload", func() {
				appWorkload := korifiv1alpha1.AppWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cfProcess.Namespace,
						Name:      appWorkloadName,
					},
				}

				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(&appWorkload), &appWorkload)).To(Succeed())
				}, "1s").Should(Succeed())
			})
		})
	})

	When("a CFRoute destination specifying a different port already exists before the app is started", func() {
		JustBeforeEach(func() {
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
				GUID:        "destination2-guid",
				AppRef:      corev1.LocalObjectReference{Name: cfApp.Name},
				ProcessType: processTypeWeb,
				Port:        port9000,
				Protocol:    "http1",
			}
			cfRoute := &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfDomain.Name,
					Namespace: cfSpace.Status.GUID,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "test-host",
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
					CurrentStatus: "valid",
					Description:   "ok",
					Destinations:  []korifiv1alpha1.Destination{destination},
				}
			})).To(Succeed())

			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an AppWorkload with VCAP env set according to the destination port", func() {
			eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				g.Expect(appWorkload.Spec.Env).To(ContainElements(
					Equal(corev1.EnvVar{Name: "VCAP_APP_PORT", Value: "9000"}),
					Equal(corev1.EnvVar{Name: "PORT", Value: "9000"}),
				))
			})
		})

		When("the process has a health check", func() {
			JustBeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
					cfProcess.Spec.HealthCheck.Type = "http"
					cfProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds = 3
					cfProcess.Spec.HealthCheck.Data.TimeoutSeconds = 31
					cfProcess.Spec.Ports = []int32{}
				})).To(Succeed())
			})

			It("eventually sets the correct health check port on the AppWorkload", func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					g.Expect(appWorkload.Spec.StartupProbe).ToNot(BeNil())
					g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Port.IntValue()).To(BeEquivalentTo(9000))
					g.Expect(appWorkload.Spec.StartupProbe.PeriodSeconds).To(BeEquivalentTo(2))
					g.Expect(appWorkload.Spec.StartupProbe.TimeoutSeconds).To(BeEquivalentTo(3))
					g.Expect(appWorkload.Spec.StartupProbe.FailureThreshold).To(BeEquivalentTo(16))

					g.Expect(appWorkload.Spec.LivenessProbe).NotTo(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe.HTTPGet).NotTo(BeNil())
					g.Expect(appWorkload.Spec.LivenessProbe.HTTPGet.Port.IntValue()).To(BeEquivalentTo(9000))
					g.Expect(appWorkload.Spec.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(30))
					g.Expect(appWorkload.Spec.LivenessProbe.TimeoutSeconds).To(BeEquivalentTo(3))
					g.Expect(appWorkload.Spec.LivenessProbe.FailureThreshold).To(BeEquivalentTo(1))
				})
			})
		})
	})

	When("the CFProcess has an http health check", func() {
		const (
			healthCheckEndpoint                 = "/healthy"
			healthCheckPort                     = 8080
			healthCheckTimeoutSeconds           = 9
			healthCheckInvocationTimeoutSeconds = 3
		)

		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		JustBeforeEach(func() {
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
			eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				g.Expect(appWorkload.Spec.StartupProbe).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.HTTPGet).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Path).To(Equal("/healthy"))
				g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Port.IntValue()).To(Equal(8080))
				g.Expect(appWorkload.Spec.StartupProbe.InitialDelaySeconds).To(BeZero())
				g.Expect(appWorkload.Spec.StartupProbe.PeriodSeconds).To(BeEquivalentTo(2))
				g.Expect(appWorkload.Spec.StartupProbe.TimeoutSeconds).To(BeEquivalentTo(3))
				g.Expect(appWorkload.Spec.StartupProbe.FailureThreshold).To(BeEquivalentTo(5))

				g.Expect(appWorkload.Spec.LivenessProbe).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.HTTPGet).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Path).To(Equal("/healthy"))
				g.Expect(appWorkload.Spec.StartupProbe.HTTPGet.Port.IntValue()).To(Equal(8080))
				g.Expect(appWorkload.Spec.LivenessProbe.InitialDelaySeconds).To(BeZero())
				g.Expect(appWorkload.Spec.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(30))
				g.Expect(appWorkload.Spec.LivenessProbe.TimeoutSeconds).To(BeEquivalentTo(3))
				g.Expect(appWorkload.Spec.LivenessProbe.FailureThreshold).To(BeEquivalentTo(1))
			})
		})
	})

	When("the CFProcess has a port health check", func() {
		const (
			healthCheckPort                     = 8080
			healthCheckTimeoutSeconds           = 9
			healthCheckInvocationTimeoutSeconds = 3
		)

		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		JustBeforeEach(func() {
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
			eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				g.Expect(appWorkload.Spec.StartupProbe).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.TCPSocket).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.TCPSocket.Port.IntValue()).To(Equal(8080))
				g.Expect(appWorkload.Spec.StartupProbe.InitialDelaySeconds).To(BeZero())
				g.Expect(appWorkload.Spec.StartupProbe.PeriodSeconds).To(BeEquivalentTo(2))
				g.Expect(appWorkload.Spec.StartupProbe.TimeoutSeconds).To(BeEquivalentTo(3))
				g.Expect(appWorkload.Spec.StartupProbe.FailureThreshold).To(BeEquivalentTo(5))

				g.Expect(appWorkload.Spec.LivenessProbe).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.TCPSocket).ToNot(BeNil())
				g.Expect(appWorkload.Spec.StartupProbe.TCPSocket.Port.IntValue()).To(Equal(8080))
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
		})

		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{Type: "process"}
			})).To(Succeed())
		})

		It("sets the liveness and readinessProbes correctly on the AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
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
