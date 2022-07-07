package integration_test

import (
	"context"
	"crypto/sha1"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

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
		ctx := context.Background()

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
		Expect(
			k8sClient.Create(ctx, cfProcess),
		).To(Succeed())

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		UpdateCFAppWithCurrentDropletRef(cfApp, testBuildGUID)
		cfApp.Spec.EnvSecretName = testAppGUID + "-env"

		appEnvSecret := BuildCFAppEnvVarsSecret(testAppGUID, testNamespace, map[string]string{"test-env-key": "test-env-val"})
		Expect(
			k8sClient.Create(ctx, appEnvSecret),
		).To(Succeed())

		cfPackage = BuildCFPackageCRObject(testPackageGUID, testNamespace, testAppGUID)
		Expect(
			k8sClient.Create(ctx, cfPackage),
		).To(Succeed())
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
		Expect(k8sClient.Delete(context.Background(), ns)).To(Succeed())
	})

	When("the CFProcess is created", func() {
		BeforeEach(func() {
			ctx := context.Background()
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually reconciles to set owner references on CFProcess", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFProcess korifiv1alpha1.CFProcess
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &createdCFProcess)
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
			ctx := context.Background()
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an RunWorkload", func() {
			Eventually(func(g Gomega) {
				ctx := context.Background()
				var runWorkloads korifiv1alpha1.RunWorkloadList
				err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
				g.Expect(err).NotTo(HaveOccurred())

				processRunWorkloads := []korifiv1alpha1.RunWorkload{}
				for _, currentRunWorkload := range runWorkloads.Items {
					if valueForKey(currentRunWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
						processRunWorkloads = append(processRunWorkloads, currentRunWorkload)
					}
				}
				g.Expect(processRunWorkloads).To(HaveLen(1))
				runWorkload := processRunWorkloads[0]

				var updatedCFApp korifiv1alpha1.CFApp
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cfApp.Name, Namespace: cfApp.Namespace}, &updatedCFApp)).To(Succeed())

				g.Expect(runWorkload.OwnerReferences).To(HaveLen(1), "expected length of ownerReferences to be 1")
				g.Expect(runWorkload.OwnerReferences[0].Name).To(Equal(cfProcess.Name))

				g.Expect(runWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFAppGUIDLabelKey, testAppGUID))
				g.Expect(runWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppRevisionKey, cfApp.Annotations[cfAppRevisionKey]))
				g.Expect(runWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessGUIDLabelKey, testProcessGUID))
				g.Expect(runWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessTypeLabelKey, cfProcess.Spec.ProcessType))

				g.Expect(runWorkload.Spec.GUID).To(Equal(cfProcess.Name), "Expected runWorkload spec GUID to match cfProcess GUID")
				g.Expect(runWorkload.Spec.Version).To(Equal(cfApp.Annotations[cfAppRevisionKey]), "Expected runWorkload version to match cfApp's app-rev annotation")
				g.Expect(runWorkload.Spec.DiskMiB).To(Equal(cfProcess.Spec.DiskQuotaMB), "runWorkload DiskMB does not match")
				g.Expect(runWorkload.Spec.MemoryMiB).To(Equal(cfProcess.Spec.MemoryMB), "runWorkload MemoryMB does not match")
				g.Expect(runWorkload.Spec.Image).To(Equal(cfBuild.Status.Droplet.Registry.Image), "runWorkload Image does not match Droplet")
				g.Expect(runWorkload.Spec.ImagePullSecrets).To(Equal(cfBuild.Status.Droplet.Registry.ImagePullSecrets), "runWorkload ImagePullSecrets does not match Droplet")
				g.Expect(runWorkload.Spec.ProcessType).To(Equal(processTypeWeb), "runWorkload process type does not match")
				g.Expect(runWorkload.Spec.AppGUID).To(Equal(cfApp.Name), "runWorkload app GUID does not match CFApp")
				g.Expect(runWorkload.Spec.Ports).To(Equal(cfProcess.Spec.Ports), "runWorkload ports do not match")
				g.Expect(runWorkload.Spec.Instances).To(Equal(int32(cfProcess.Spec.DesiredInstances)), "runWorkload desired instances does not match CFApp")
				g.Expect(runWorkload.Spec.CPUWeight).NotTo(BeZero(), "expected cpu to be nonzero")
				g.Expect(runWorkload.Spec.Env).To(ConsistOf(
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
				g.Expect(runWorkload.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", processTypeWebCommand))
			}).Should(Succeed(), fmt.Sprintf("Timed out waiting for expected RunWorkload/%s in namespace %s to be created", testProcessGUID, testNamespace))
		})

		When("a CFApp desired state is updated to STOPPED", func() {
			JustBeforeEach(func() {
				ctx := context.Background()

				// Wait for RunWorkload to exist before updating CFApp
				Eventually(func() string {
					var runWorkloads korifiv1alpha1.RunWorkloadList
					err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
					if err != nil {
						return ""
					}

					for _, currentRunWorkload := range runWorkloads.Items {
						if valueForKey(currentRunWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
							return currentRunWorkload.GetName()
						}
					}

					return ""
				}).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for RunWorkload/%s in namespace %s to be created", testProcessGUID, testNamespace))

				originalCFApp := cfApp.DeepCopy()
				cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				Expect(k8sClient.Patch(ctx, cfApp, client.MergeFrom(originalCFApp))).To(Succeed())
			})

			It("eventually deletes the RunWorkloads", func() {
				ctx := context.Background()

				Eventually(func() bool {
					var runWorkloads korifiv1alpha1.RunWorkloadList
					err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
					if err != nil {
						return false
					}

					for _, currentRunWorkload := range runWorkloads.Items {
						if valueForKey(currentRunWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
							return false
						}
					}

					return true
				}).Should(BeTrue(), "Timed out waiting for deletion of RunWorkload/%s in namespace %s to cause NotFound error", testProcessGUID, testNamespace)
			})
		})
	})

	When("a CFRoute destination specifying a different port already exists before the app is started", func() {
		var runWorkload korifiv1alpha1.RunWorkload

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
			Expect(k8sClient.Create(context.Background(), cfRoute)).To(Succeed())
			cfRoute.Status = korifiv1alpha1.CFRouteStatus{
				CurrentStatus: "valid",
				Description:   "ok",
				Destinations:  []korifiv1alpha1.Destination{wrongDestination, destination},
			}
			Expect(k8sClient.Status().Update(context.Background(), cfRoute)).To(Succeed())
		})

		JustBeforeEach(func() {
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(context.Background(), cfApp),
			).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an RunWorkload with VCAP env set according to the destination port", func() {
			Eventually(func() string {
				var runWorkloads korifiv1alpha1.RunWorkloadList
				err := k8sClient.List(context.Background(), &runWorkloads, client.InNamespace(testNamespace))
				if err != nil {
					return ""
				}

				for _, currentRunWorkload := range runWorkloads.Items {
					if valueForKey(currentRunWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
						runWorkload = currentRunWorkload
						return runWorkload.GetName()
					}
				}

				return ""
			}).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for RunWorkload/%s in namespace %s to be created", testProcessGUID, testNamespace))

			Expect(runWorkload.Spec.Env).To(ContainElements(
				Equal(corev1.EnvVar{Name: "VCAP_APP_PORT", Value: "9000"}),
				Equal(corev1.EnvVar{Name: "PORT", Value: "9000"}),
			))
		})

		When("the process has a health check", func() {
			BeforeEach(func() {
				ctx := context.Background()
				cfProcess.Spec.HealthCheck.Type = "process"
				cfProcess.Spec.Ports = []int32{}
				Expect(k8sClient.Update(ctx, cfProcess)).To(Succeed())
			})

			It("eventually sets the correct health check port on the RunWorkload", func() {
				ctx := context.Background()
				var runWorkload korifiv1alpha1.RunWorkload

				Eventually(func() string {
					var runWorkloads korifiv1alpha1.RunWorkloadList
					err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
					if err != nil {
						return ""
					}

					for _, currentRunWorkload := range runWorkloads.Items {
						if valueForKey(currentRunWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
							runWorkload = currentRunWorkload
							return currentRunWorkload.GetName()
						}
					}

					return ""
				}).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for RunWorkload/%s in namespace %s to be created", testProcessGUID, testNamespace))

				Expect(runWorkload.Spec.Health.Type).To(Equal(string(cfProcess.Spec.HealthCheck.Type)))
				Expect(runWorkload.Spec.Health.Port).To(BeEquivalentTo(9000))
			})
		})
	})

	When("the CFApp has an RunWorkload and is restarted by bumping the \"rev\" annotation", func() {
		var newRevValue string
		BeforeEach(func() {
			ctx := context.Background()

			appRev := cfApp.Annotations[cfAppRevisionKey]
			h := sha1.New()
			h.Write([]byte(appRev))
			appRevHash := h.Sum(nil)
			runWorkload1Name := testProcessGUID + fmt.Sprintf("-%x", appRevHash)[:5]
			// Create the rev1 RunWorkload
			rev1RunWorkload := &korifiv1alpha1.RunWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      runWorkload1Name,
					Namespace: testNamespace,
					Labels: map[string]string{
						CFAppGUIDLabelKey:     cfApp.Name,
						cfAppRevisionKey:      appRev,
						CFProcessGUIDLabelKey: testProcessGUID,
						CFProcessTypeLabelKey: cfProcess.Spec.ProcessType,
					},
				},
				Spec: korifiv1alpha1.RunWorkloadSpec{
					MemoryMiB:        1,
					DiskMiB:          1,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "some-image-pull-secret"}},
				},
			}
			Expect(
				k8sClient.Create(ctx, rev1RunWorkload),
			).To(Succeed())

			// Set CFApp annotation.rev 0->1 & set state to started to trigger reconcile
			newRevValue = "1"
			cfApp.Annotations[cfAppRevisionKey] = newRevValue
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("deletes the old RunWorkload, and creates a new RunWorkload for the current revision", func() {
			// check that RunWorkload rev1 is eventually created
			ctx := context.Background()

			var rev2RunWorkload *korifiv1alpha1.RunWorkload
			Eventually(func() bool {
				var runWorkloads korifiv1alpha1.RunWorkloadList
				err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
				if err != nil {
					return false
				}

				for _, currentRunWorkload := range runWorkloads.Items {
					if processGUIDLabel, has := currentRunWorkload.Labels[CFProcessGUIDLabelKey]; has && processGUIDLabel == testProcessGUID {
						if revLabel, has := currentRunWorkload.Labels[cfAppRevisionKey]; has && revLabel == newRevValue {
							rev2RunWorkload = &currentRunWorkload
							return true
						}
					}
				}
				return false
			}).Should(BeTrue(), "Timed out waiting for creation of RunWorkload/%s in namespace %s", testProcessGUID, testNamespace)

			Expect(rev2RunWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFAppGUIDLabelKey, testAppGUID))
			Expect(rev2RunWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppRevisionKey, cfApp.Annotations[cfAppRevisionKey]))
			Expect(rev2RunWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessGUIDLabelKey, testProcessGUID))
			Expect(rev2RunWorkload.ObjectMeta.Labels).To(HaveKeyWithValue(CFProcessTypeLabelKey, cfProcess.Spec.ProcessType))

			// check that RunWorkload rev0 is eventually deleted
			Eventually(func() bool {
				var runWorkloads korifiv1alpha1.RunWorkloadList
				err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
				if err != nil {
					return true
				}

				for _, currentRunWorkload := range runWorkloads.Items {
					if processGUIDLabel, has := currentRunWorkload.Labels[CFProcessGUIDLabelKey]; has && processGUIDLabel == testProcessGUID {
						if revLabel, has := currentRunWorkload.Labels[cfAppRevisionKey]; has && revLabel == korifiv1alpha1.CFAppRevisionKeyDefault {
							return true
						}
					}
				}
				return false
			}).Should(BeFalse(), "Timed out waiting for deletion of RunWorkload/%s in namespace %s", testProcessGUID, testNamespace)
		})
	})

	When("the CFProcess has a health check", func() {
		BeforeEach(func() {
			ctx := context.Background()

			cfProcess.Spec.HealthCheck.Type = "process"
			cfProcess.Spec.Ports = []int32{}
			Expect(
				k8sClient.Update(ctx, cfProcess),
			).To(Succeed())

			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an RunWorkload", func() {
			ctx := context.Background()
			var runWorkload korifiv1alpha1.RunWorkload

			Eventually(func() string {
				var runWorkloads korifiv1alpha1.RunWorkloadList
				err := k8sClient.List(ctx, &runWorkloads, client.InNamespace(testNamespace))
				if err != nil {
					return ""
				}

				for _, currentRunWorkload := range runWorkloads.Items {
					if valueForKey(currentRunWorkload.Labels, korifiv1alpha1.CFProcessGUIDLabelKey) == testProcessGUID {
						runWorkload = currentRunWorkload
						return currentRunWorkload.GetName()
					}
				}

				return ""
			}).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for RunWorkload/%s in namespace %s to be created", testProcessGUID, testNamespace))

			Expect(runWorkload.Spec.Health.Type).To(Equal(string(cfProcess.Spec.HealthCheck.Type)))
			Expect(runWorkload.Spec.Health.Port).To(BeEquivalentTo(8080))
		})
	})
})
