package workloads_test

import (
	"context"
	"crypto/sha1"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
		oldDetectedCommand       = "old-detected-command"
		newDetectedCommand       = "new-detected-command"
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
		cfProcess = BuildCFProcessCRObject(testProcessGUID, cfSpace.Status.GUID, testAppGUID, processTypeWeb, processTypeWebCommand, oldDetectedCommand)
		cfApp = BuildCFAppCRObject(testAppGUID, cfSpace.Status.GUID)
		cfPackage = BuildCFPackageCRObject(testPackageGUID, cfSpace.Status.GUID, testAppGUID)
		cfBuild = BuildCFBuildObject(testBuildGUID, cfSpace.Status.GUID, testPackageGUID, testAppGUID)
	})
	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

		UpdateCFAppWithCurrentDropletRef(cfApp, testBuildGUID)
		cfApp.Spec.EnvSecretName = testAppGUID + "-env"

		appEnvSecret := BuildCFAppEnvVarsSecret(testAppGUID, cfSpace.Status.GUID, map[string]string{"test-env-key": "test-env-val"})
		Expect(k8sClient.Create(ctx, appEnvSecret)).To(Succeed())

		Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
		dropletProcessTypeMap := map[string]string{
			processTypeWeb:    newDetectedCommand,
			processTypeWorker: processTypeWorkerCommand,
		}
		dropletPorts := []int32{port8080, port9000}
		buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypeMap, dropletPorts)
		cfBuild = createBuildWithDroplet(ctx, k8sClient, cfBuild, buildDropletStatus)
	})

	When("the CFProcess is created", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("eventually reconciles to set owner references on CFProcess", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFProcess korifiv1alpha1.CFProcess
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: cfSpace.Status.GUID}, &createdCFProcess)
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
	})

	When("the CFApp desired state is STARTED", func() {
		JustBeforeEach(func() {
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an AppWorkload", func() {
			eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
				var updatedCFApp korifiv1alpha1.CFApp
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), &updatedCFApp)).To(Succeed())

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

				g.Expect(appWorkload.Spec.Env).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("test-env-key"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "test-env-key",
							})),
						})),
					}),
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
					Equal(corev1.EnvVar{Name: "PORT", Value: "8080"}),
				))
				g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", processTypeWebCommand))
			})
		})

		When("The process command field isn't set", func() {
			BeforeEach(func() {
				cfProcess.Spec.Command = ""
			})

			It("eventually creates an app workload using the newDetectedCommand from the build", func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {
					var updatedCFApp korifiv1alpha1.CFApp
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), &updatedCFApp)).To(Succeed())

					g.Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", newDetectedCommand))
				})
			})
		})

		When("a CFApp desired state is updated to STOPPED", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
					cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				})).To(Succeed())
			})

			It("eventually deletes the AppWorkloads", func() {
				Eventually(func() ([]korifiv1alpha1.AppWorkload, error) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(cfSpace.Status.GUID), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
					})
					return appWorkloads.Items, err
				}).Should(BeEmpty(), "Timed out waiting for deletion of AppWorkload/%s in namespace %s to cause NotFound error", testProcessGUID, cfSpace.Status.GUID)
			})
		})

		When("the app process instances are scaled down to 0", func() {
			JustBeforeEach(func() {
				eventuallyCreatedAppWorkloadShould(testProcessGUID, cfSpace.Status.GUID, func(g Gomega, appWorkload korifiv1alpha1.AppWorkload) {})
				Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = tools.PtrTo(0)
				})).To(Succeed())
			})

			It("deletes the app workload", func() {
				Eventually(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := k8sClient.List(context.Background(), &appWorkloads, client.InNamespace(cfProcess.Namespace), client.MatchingLabels{
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
				Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
					cfProcess.Spec.DesiredInstances = nil
				})).To(Succeed())
			})

			It("does not delete the app workload", func() {
				Consistently(func(g Gomega) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := k8sClient.List(context.Background(), &appWorkloads, client.InNamespace(cfProcess.Namespace), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
					})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(appWorkloads.Items).NotTo(BeEmpty())
				}).Should(Succeed())
			})
		})
	})

	When("a CFRoute destination specifying a different port already exists before the app is started", func() {
		JustBeforeEach(func() {
			wrongDestination := korifiv1alpha1.Destination{
				GUID:        "destination1-guid",
				AppRef:      corev1.LocalObjectReference{Name: "some-other-guid"},
				ProcessType: processTypeWeb,
				Port:        12,
				Protocol:    "http1",
			}
			destination := korifiv1alpha1.Destination{
				GUID:        "destination2-guid",
				AppRef:      corev1.LocalObjectReference{Name: cfApp.Name},
				ProcessType: processTypeWeb,
				Port:        port9000,
				Protocol:    "http1",
			}
			cfRoute := &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateGUID(),
					Namespace: cfSpace.Status.GUID,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "test-host",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      GenerateGUID(),
						Namespace: cfSpace.Status.GUID,
					},
					Destinations: []korifiv1alpha1.Destination{wrongDestination, destination},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
			Expect(k8s.Patch(ctx, k8sClient, cfRoute, func() {
				cfRoute.Status = korifiv1alpha1.CFRouteStatus{
					CurrentStatus: "valid",
					Description:   "ok",
					Destinations:  []korifiv1alpha1.Destination{wrongDestination, destination},
				}
			})).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
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
				Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
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

	When("the CFApp has an AppWorkload and is restarted by bumping the \"rev\" annotation", func() {
		var newRevValue string
		JustBeforeEach(func() {
			appRev := cfApp.Annotations[cfAppRevisionKey]
			h := sha1.New()
			h.Write([]byte(appRev))
			appRevHash := h.Sum(nil)
			appWorkload1Name := testProcessGUID + fmt.Sprintf("-%x", appRevHash)[:5]
			// Create the rev1 AppWorkload
			rev1AppWorkload := &korifiv1alpha1.AppWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appWorkload1Name,
					Namespace: cfSpace.Status.GUID,
					Labels: map[string]string{
						CFAppGUIDLabelKey:     cfApp.Name,
						cfAppRevisionKey:      appRev,
						CFProcessGUIDLabelKey: testProcessGUID,
						CFProcessTypeLabelKey: cfProcess.Spec.ProcessType,
					},
				},
				Spec: korifiv1alpha1.AppWorkloadSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "some-image-pull-secret"}},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory:           resource.MustParse("1Mi"),
							corev1.ResourceEphemeralStorage: resource.MustParse("1Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Mi"),
						},
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, rev1AppWorkload),
			).To(Succeed())

			// Set CFApp annotation.rev 0->1 & set state to started to trigger reconcile
			newRevValue = "1"
			cfApp.Annotations[cfAppRevisionKey] = newRevValue
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("deletes the old AppWorkload, and creates a new AppWorkload for the current revision", func() {
			// check that AppWorkload rev1 is eventually created
			var rev2AppWorkload *korifiv1alpha1.AppWorkload
			Eventually(func() bool {
				var appWorkloads korifiv1alpha1.AppWorkloadList
				err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(cfSpace.Status.GUID))
				if err != nil {
					return false
				}

				for _, currentAppWorkload := range appWorkloads.Items {
					if processGUIDLabel, has := currentAppWorkload.Labels[CFProcessGUIDLabelKey]; has && processGUIDLabel == testProcessGUID {
						if revLabel, has := currentAppWorkload.Labels[cfAppRevisionKey]; has && revLabel == newRevValue {
							rev2AppWorkload = &currentAppWorkload
							return true
						}
					}
				}
				return false
			}).Should(BeTrue(), "Timed out waiting for creation of AppWorkload/%s in namespace %s", testProcessGUID, cfSpace.Status.GUID)

			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFAppGUIDLabelKey, testAppGUID))
			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppRevisionKey, cfApp.Annotations[cfAppRevisionKey]))
			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessGUIDLabelKey, testProcessGUID))
			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessTypeLabelKey, cfProcess.Spec.ProcessType))

			// check that AppWorkload rev0 is eventually deleted
			Eventually(func() bool {
				var appWorkloads korifiv1alpha1.AppWorkloadList
				err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(cfSpace.Status.GUID))
				if err != nil {
					return true
				}

				for _, currentAppWorkload := range appWorkloads.Items {
					if processGUIDLabel, has := currentAppWorkload.Labels[CFProcessGUIDLabelKey]; has && processGUIDLabel == testProcessGUID {
						if revLabel, has := currentAppWorkload.Labels[cfAppRevisionKey]; has && revLabel == korifiv1alpha1.CFAppRevisionKeyDefault {
							return true
						}
					}
				}
				return false
			}).Should(BeFalse(), "Timed out waiting for deletion of AppWorkload/%s in namespace %s", testProcessGUID, cfSpace.Status.GUID)
		})
	})

	When("the CFProcess has an http health check", func() {
		const (
			healthCheckEndpoint                 = "/healthy"
			healthCheckPort                     = 8080
			healthCheckTimeoutSeconds           = 9
			healthCheckInvocationTimeoutSeconds = 3
		)
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
					Type: "http",
					Data: korifiv1alpha1.HealthCheckData{
						HTTPEndpoint:             healthCheckEndpoint,
						InvocationTimeoutSeconds: healthCheckInvocationTimeoutSeconds,
						TimeoutSeconds:           healthCheckTimeoutSeconds,
					},
				}
			})).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
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
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
					Type: "port",
					Data: korifiv1alpha1.HealthCheckData{
						InvocationTimeoutSeconds: healthCheckInvocationTimeoutSeconds,
						TimeoutSeconds:           healthCheckTimeoutSeconds,
					},
				}
			})).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
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
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
				cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{Type: "process"}
			})).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
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
	EventuallyWithOffset(1, func(g Gomega) {
		var appWorkloads korifiv1alpha1.AppWorkloadList
		err := k8sClient.List(context.Background(), &appWorkloads, client.InNamespace(namespace), client.MatchingLabels{
			korifiv1alpha1.CFProcessGUIDLabelKey: processGUID,
		})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(appWorkloads.Items).To(HaveLen(1))

		shouldFn(g, appWorkloads.Items[0])
	}).Should(Succeed())
}
