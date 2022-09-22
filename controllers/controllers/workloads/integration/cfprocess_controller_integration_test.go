package integration_test

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tests/matchers"

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
		testNamespace string
		ns            *corev1.Namespace

		testProcessGUID string
		testAppGUID     string
		testBuildGUID   string
		testPackageGUID string
		cfProcess       *korifiv1alpha1.CFProcess
		cfPackage       *korifiv1alpha1.CFPackage
		cfApp           *korifiv1alpha1.CFApp
		cfBuild         *korifiv1alpha1.CFBuild

		ctx context.Context
	)

	const (
		CFAppGUIDLabelKey     = "korifi.cloudfoundry.org/app-guid"
		cfAppRevisionKey      = "korifi.cloudfoundry.org/app-rev"
		CFProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
		CFProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"

		processTypeWeb           = "web"
		processTypeWebCommand    = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
		processTypeWorker        = "worker"
		processTypeWorkerCommand = "bundle exec rackup config.ru"
		port8080                 = 8080
		port9000                 = 9000
	)

	BeforeEach(func() {
		ctx = context.Background()

		testNamespace = GenerateGUID()
		ns = createNamespace(ctx, k8sClient, testNamespace)

		testAppGUID = GenerateGUID()
		testProcessGUID = GenerateGUID()
		testBuildGUID = GenerateGUID()
		testPackageGUID = GenerateGUID()

		// Technically the app controller should be creating this process based on CFApp and CFBuild, but we
		// want to drive testing with a specific CFProcess instead of cascading (non-object-ref) state through
		// other resources.
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, processTypeWeb, processTypeWebCommand)
		Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		UpdateCFAppWithCurrentDropletRef(cfApp, testBuildGUID)
		cfApp.Spec.EnvSecretName = testAppGUID + "-env"

		appEnvSecret := BuildCFAppEnvVarsSecret(testAppGUID, testNamespace, map[string]string{"test-env-key": "test-env-val"})
		Expect(k8sClient.Create(ctx, appEnvSecret)).To(Succeed())

		cfPackage = BuildCFPackageCRObject(testPackageGUID, testNamespace, testAppGUID)
		Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		dropletProcessTypeMap := map[string]string{
			processTypeWeb:    processTypeWebCommand,
			processTypeWorker: processTypeWorkerCommand,
		}
		dropletPorts := []int32{port8080, port9000}
		buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypeMap, dropletPorts)
		cfBuild = createBuildWithDroplet(ctx, k8sClient, cfBuild, buildDropletStatus)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	When("the CFProcess is created", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("eventually reconciles to set owner references on CFProcess", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFProcess korifiv1alpha1.CFProcess
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &createdCFProcess)
				if err != nil {
					return nil
				}
				return createdCFProcess.GetOwnerReferences()
			}).Should(ConsistOf(metav1.OwnerReference{
				APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				Kind:       "CFApp",
				Name:       cfApp.Name,
				UID:        cfApp.UID,
			}))
		})
	})

	When("the CFApp desired state is STARTED", func() {
		JustBeforeEach(func() {
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an AppWorkload", func() {
			appWorkload := eventuallyCreatedAppWorkload(testProcessGUID, testNamespace)

			var updatedCFApp korifiv1alpha1.CFApp
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), &updatedCFApp)).To(Succeed())

			Expect(appWorkload.OwnerReferences).To(HaveLen(1), "expected length of ownerReferences to be 1")
			Expect(appWorkload.OwnerReferences[0].Name).To(Equal(cfProcess.Name))

			Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFAppGUIDLabelKey, testAppGUID))
			Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppRevisionKey, cfApp.Annotations[cfAppRevisionKey]))
			Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessGUIDLabelKey, testProcessGUID))
			Expect(appWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessTypeLabelKey, cfProcess.Spec.ProcessType))

			Expect(appWorkload.Spec.GUID).To(Equal(cfProcess.Name))
			Expect(appWorkload.Spec.Version).To(Equal(cfApp.Annotations[cfAppRevisionKey]))
			Expect(appWorkload.Spec.Image).To(Equal(cfBuild.Status.Droplet.Registry.Image))
			Expect(appWorkload.Spec.ImagePullSecrets).To(Equal(cfBuild.Status.Droplet.Registry.ImagePullSecrets))
			Expect(appWorkload.Spec.ProcessType).To(Equal(processTypeWeb))
			Expect(appWorkload.Spec.AppGUID).To(Equal(cfApp.Name))
			Expect(appWorkload.Spec.Ports).To(Equal(cfProcess.Spec.Ports))
			Expect(appWorkload.Spec.Instances).To(Equal(int32(*cfProcess.Spec.DesiredInstances)))

			Expect(appWorkload.Spec.Resources.Limits.StorageEphemeral()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.DiskQuotaMB, "Mi"))
			Expect(appWorkload.Spec.Resources.Limits.Memory()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.MemoryMB, "Mi"))
			Expect(appWorkload.Spec.Resources.Requests.StorageEphemeral()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.DiskQuotaMB, "Mi"))
			Expect(appWorkload.Spec.Resources.Requests.Memory()).To(matchers.RepresentResourceQuantity(cfProcess.Spec.MemoryMB, "Mi"))
			Expect(appWorkload.Spec.Resources.Requests.Cpu()).To(matchers.RepresentResourceQuantity(100, "m"), "expected cpu request to be 100m (Based on 1024MiB memory)")

			Expect(appWorkload.Spec.Env).To(ConsistOf(
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
				Equal(corev1.EnvVar{Name: "VCAP_APP_HOST", Value: "0.0.0.0"}),
				Equal(corev1.EnvVar{Name: "VCAP_APP_PORT", Value: "8080"}),
				Equal(corev1.EnvVar{Name: "PORT", Value: "8080"}),
			))
			Expect(appWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", processTypeWebCommand))
		})

		When("a CFApp desired state is updated to STOPPED", func() {
			JustBeforeEach(func() {
				_ = eventuallyCreatedAppWorkload(testProcessGUID, testNamespace)

				originalCFApp := cfApp.DeepCopy()
				cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				Expect(k8sClient.Patch(ctx, cfApp, client.MergeFrom(originalCFApp))).To(Succeed())
			})

			It("eventually deletes the AppWorkloads", func() {
				Eventually(func() ([]korifiv1alpha1.AppWorkload, error) {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace), client.MatchingLabels{
						korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
					})
					return appWorkloads.Items, err
				}).Should(BeEmpty(), "Timed out waiting for deletion of AppWorkload/%s in namespace %s to cause NotFound error", testProcessGUID, testNamespace)
			})
		})
	})

	When("a CFRoute destination specifying a different port already exists before the app is started", func() {
		BeforeEach(func() {
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
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "test-host",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      GenerateGUID(),
						Namespace: testNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{wrongDestination, destination},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
			cfRoute.Status = korifiv1alpha1.CFRouteStatus{
				CurrentStatus: "valid",
				Description:   "ok",
				Destinations:  []korifiv1alpha1.Destination{wrongDestination, destination},
			}
			Expect(k8sClient.Status().Update(ctx, cfRoute)).To(Succeed())
		})

		JustBeforeEach(func() {
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an AppWorkload with VCAP env set according to the destination port", func() {
			appWorkload := eventuallyCreatedAppWorkload(testProcessGUID, testNamespace)

			Expect(appWorkload.Spec.Env).To(ContainElements(
				Equal(corev1.EnvVar{Name: "VCAP_APP_PORT", Value: "9000"}),
				Equal(corev1.EnvVar{Name: "PORT", Value: "9000"}),
			))
		})

		When("the process has a health check", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck.Type = "http"
				cfProcess.Spec.Ports = []int32{}
				Expect(k8sClient.Update(ctx, cfProcess)).To(Succeed())
			})

			It("eventually sets the correct health check port on the AppWorkload", func() {
				var appWorkload korifiv1alpha1.AppWorkload

				Eventually(func() string {
					var appWorkloads korifiv1alpha1.AppWorkloadList
					err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace))
					if err != nil {
						return ""
					}

					for _, currentAppWorkload := range appWorkloads.Items {
						if valueForKey(currentAppWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
							appWorkload = currentAppWorkload
							return currentAppWorkload.GetName()
						}
					}

					return ""
				}).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for AppWorkload/%s in namespace %s to be created", testProcessGUID, testNamespace))

				Expect(appWorkload.Spec.LivenessProbe).NotTo(BeNil())
				Expect(appWorkload.Spec.LivenessProbe.HTTPGet).NotTo(BeNil())
				Expect(appWorkload.Spec.LivenessProbe.HTTPGet.Port.IntValue()).To(BeEquivalentTo(9000))

				Expect(appWorkload.Spec.ReadinessProbe).NotTo(BeNil())
				Expect(appWorkload.Spec.ReadinessProbe.HTTPGet).NotTo(BeNil())
				Expect(appWorkload.Spec.ReadinessProbe.HTTPGet.Port.IntValue()).To(BeEquivalentTo(9000))
			})
		})
	})

	When("the CFApp has an AppWorkload and is restarted by bumping the \"rev\" annotation", func() {
		var newRevValue string
		BeforeEach(func() {
			appRev := cfApp.Annotations[cfAppRevisionKey]
			h := sha1.New()
			h.Write([]byte(appRev))
			appRevHash := h.Sum(nil)
			appWorkload1Name := testProcessGUID + fmt.Sprintf("-%x", appRevHash)[:5]
			// Create the rev1 AppWorkload
			rev1AppWorkload := &korifiv1alpha1.AppWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appWorkload1Name,
					Namespace: testNamespace,
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
				err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace))
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
			}).Should(BeTrue(), "Timed out waiting for creation of AppWorkload/%s in namespace %s", testProcessGUID, testNamespace)

			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFAppGUIDLabelKey, testAppGUID))
			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppRevisionKey, cfApp.Annotations[cfAppRevisionKey]))
			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessGUIDLabelKey, testProcessGUID))
			Expect(rev2AppWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessTypeLabelKey, cfProcess.Spec.ProcessType))

			// check that AppWorkload rev0 is eventually deleted
			Eventually(func() bool {
				var appWorkloads korifiv1alpha1.AppWorkloadList
				err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(testNamespace))
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
			}).Should(BeFalse(), "Timed out waiting for deletion of AppWorkload/%s in namespace %s", testProcessGUID, testNamespace)
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
			cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
				Type: "http",
				Data: korifiv1alpha1.HealthCheckData{
					HTTPEndpoint:             healthCheckEndpoint,
					InvocationTimeoutSeconds: healthCheckInvocationTimeoutSeconds,
					TimeoutSeconds:           healthCheckTimeoutSeconds,
				},
			}
			Expect(k8sClient.Update(ctx, cfProcess)).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("sets the liveness and readinessProbes correctly on the AppWorkload", func() {
			appWorkload := eventuallyCreatedAppWorkload(testProcessGUID, testNamespace)

			livenessProbeJSON, err := json.Marshal(appWorkload.Spec.LivenessProbe)
			Expect(err).NotTo(HaveOccurred())
			Expect(livenessProbeJSON).To(MatchJSON(fmt.Sprintf(`{
				"httpGet": { "path": %q, "port": %d},
				"failureThreshold": %d,
				"initialDelaySeconds": %d
			}`, healthCheckEndpoint, healthCheckPort, 4, healthCheckTimeoutSeconds)))

			readinessProbeJSON, err := json.Marshal(appWorkload.Spec.ReadinessProbe)
			Expect(err).NotTo(HaveOccurred())
			Expect(readinessProbeJSON).To(MatchJSON(fmt.Sprintf(`{
				"httpGet": { "path": %q, "port": %d},
				"failureThreshold": %d
			}`, healthCheckEndpoint, healthCheckPort, 1)))
		})
	})

	When("the CFProcess has a port health check", func() {
		const (
			healthCheckPort                     = 8080
			healthCheckTimeoutSeconds           = 9
			healthCheckInvocationTimeoutSeconds = 3
		)
		BeforeEach(func() {
			cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{
				Type: "port",
				Data: korifiv1alpha1.HealthCheckData{
					InvocationTimeoutSeconds: healthCheckInvocationTimeoutSeconds,
					TimeoutSeconds:           healthCheckTimeoutSeconds,
				},
			}
			Expect(k8sClient.Update(ctx, cfProcess)).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("sets the liveness and readinessProbes correctly on the AppWorkload", func() {
			appWorkload := eventuallyCreatedAppWorkload(testProcessGUID, testNamespace)

			livenessProbeJSON, err := json.Marshal(appWorkload.Spec.LivenessProbe)
			Expect(err).NotTo(HaveOccurred())
			Expect(livenessProbeJSON).To(MatchJSON(fmt.Sprintf(`{
				"tcpSocket": {"port": %d},
				"failureThreshold": %d,
				"initialDelaySeconds": %d
			}`, healthCheckPort, 4, healthCheckTimeoutSeconds)))

			readinessProbeJSON, err := json.Marshal(appWorkload.Spec.ReadinessProbe)
			Expect(err).NotTo(HaveOccurred())
			Expect(readinessProbeJSON).To(MatchJSON(fmt.Sprintf(`{
				"tcpSocket": {"port": %d},
				"failureThreshold": %d
			}`, healthCheckPort, 1)))
		})
	})

	When("the CFProcess has a process health check", func() {
		BeforeEach(func() {
			cfProcess.Spec.HealthCheck = korifiv1alpha1.HealthCheck{Type: "process"}
			Expect(k8sClient.Update(ctx, cfProcess)).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("sets the liveness and readinessProbes correctly on the AppWorkload", func() {
			appWorkload := eventuallyCreatedAppWorkload(testProcessGUID, testNamespace)

			Expect(appWorkload.Spec.LivenessProbe).To(BeNil())
			Expect(appWorkload.Spec.ReadinessProbe).To(BeNil())
		})
	})
})

func eventuallyCreatedAppWorkload(processGUID, namespace string) korifiv1alpha1.AppWorkload {
	ctx := context.Background()
	var appWorkload korifiv1alpha1.AppWorkload

	EventuallyWithOffset(1, func(g Gomega) (*korifiv1alpha1.AppWorkload, error) {
		var appWorkloads korifiv1alpha1.AppWorkloadList
		err := k8sClient.List(ctx, &appWorkloads, client.InNamespace(namespace), client.MatchingLabels{
			korifiv1alpha1.CFProcessGUIDLabelKey: processGUID,
		})
		if err != nil {
			return nil, err
		}
		if len(appWorkloads.Items) == 0 {
			return nil, nil // retry
		}
		g.Expect(appWorkloads.Items).To(HaveLen(1))
		appWorkload = appWorkloads.Items[0]
		return &appWorkload, nil
	}).ShouldNot(BeNil(), fmt.Sprintf("Timed out waiting for AppWorkload/%s in namespace %s to be created", processGUID, namespace))
	return appWorkload
}
