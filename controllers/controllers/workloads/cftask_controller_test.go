package workloads_test

import (
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFTaskReconciler Integration Tests", func() {
	var (
		cfApp     *korifiv1alpha1.CFApp
		cfDroplet *korifiv1alpha1.CFBuild
		cfSpace   *korifiv1alpha1.CFSpace
		cfTask    *korifiv1alpha1.CFTask
		envSecret *corev1.Secret
	)

	BeforeEach(func() {
		cfSpace = createSpace(cfOrg)

		cfAppName := testutils.PrefixedGUID("app")

		cfPackage := &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      testutils.PrefixedGUID("package"),
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "bits",
				AppRef: corev1.LocalObjectReference{
					Name: cfAppName,
				},
			},
		}
		Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())

		cfDroplet = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      testutils.PrefixedGUID("droplet"),
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: corev1.LocalObjectReference{
					Name: cfPackage.Name,
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfAppName,
				},
				Lifecycle: korifiv1alpha1.Lifecycle{Type: "buildpack"},
			},
		}
		Expect(k8sClient.Create(ctx, cfDroplet)).To(Succeed())

		cfDropletCopy := cfDroplet.DeepCopy()
		cfDropletCopy.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
			Registry: korifiv1alpha1.Registry{
				Image: "registry.io/my/image",
				ImagePullSecrets: []corev1.LocalObjectReference{{
					Name: "registry-secret",
				}},
			},
			ProcessTypes: []korifiv1alpha1.ProcessType{{
				Type:    "web",
				Command: "cmd",
			}},
			Ports: []int32{8080},
		}
		meta.SetStatusCondition(&cfDropletCopy.Status.Conditions, metav1.Condition{
			Type:   "type",
			Status: "Unknown",
			Reason: "reason",
		})
		Expect(k8sClient.Status().Patch(ctx, cfDropletCopy, client.MergeFrom(cfDroplet))).To(Succeed())

		envSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.PrefixedGUID("env-secret"),
				Namespace: cfSpace.Status.GUID,
			},
			StringData: map[string]string{
				"BOB":  "flemming",
				"FAST": "show",
			},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(k8sClient.Create(ctx, envSecret)).To(Succeed())

		cfProcess := &korifiv1alpha1.CFProcess{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      testutils.PrefixedGUID("web-process"),
				Labels: map[string]string{
					korifiv1alpha1.CFProcessTypeLabelKey: "web",
					korifiv1alpha1.CFAppGUIDLabelKey:     cfAppName,
				},
			},
			Spec: korifiv1alpha1.CFProcessSpec{
				AppRef: corev1.LocalObjectReference{
					Name: cfAppName,
				},
				ProcessType: "web",
				Command:     "echo hello",
				MemoryMB:    768,
				HealthCheck: korifiv1alpha1.HealthCheck{
					Type: "process",
				},
				Ports: []int32{8080},
			},
		}
		Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      cfAppName,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				Lifecycle: korifiv1alpha1.Lifecycle{Type: "buildpack"},
				CurrentDropletRef: corev1.LocalObjectReference{
					Name: cfDroplet.Name,
				},
				DesiredState:  "STOPPED",
				DisplayName:   "app",
				EnvSecretName: envSecret.Name,
			},
		}
		Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

		cfTask = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      testutils.PrefixedGUID("cftask"),
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				Command: "echo hello",
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
		}
	})

	Describe("CFTask creation", func() {
		var (
			eventCallCount int
			task           *korifiv1alpha1.CFTask
		)

		BeforeEach(func() {
			task = &korifiv1alpha1.CFTask{}
			Eventually(func(g Gomega) {
				app := new(korifiv1alpha1.CFApp)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Status.GUID, Name: cfApp.Name}, app)).To(Succeed())

				readyCondition := meta.FindStatusCondition(app.Status.Conditions, "Ready")
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue), "App is not staged")
			}).Should(Succeed())

			eventCallCount = eventRecorder.EventfCallCount()
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfTask)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Status.GUID, Name: cfTask.Name}, task)).To(Succeed())
				initializedStatusCondition := meta.FindStatusCondition(task.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)
				g.Expect(initializedStatusCondition).NotTo(BeNil())
				g.Expect(initializedStatusCondition.Status).To(Equal(metav1.ConditionTrue), "task did not become initialized")
				g.Expect(initializedStatusCondition.Reason).To(Equal("TaskInitialized"))
				g.Expect(initializedStatusCondition.ObservedGeneration).To(Equal(task.Generation))
			}).Should(Succeed())
		})

		It("sets the app to be the task owner", func() {
			Expect(task.GetOwnerReferences()).To(ConsistOf(HaveField("Name", cfApp.Name)))
		})

		It("populates the droplet name in the status", func() {
			Expect(task.Status.DropletRef.Name).To(Equal(cfDroplet.Name))
		})

		It("sets the ObservedGeneration status field", func() {
			Expect(task.Status.ObservedGeneration).To(Equal(task.Generation))
		})

		It("creates an TaskWorkload", func() {
			var taskWorkload korifiv1alpha1.TaskWorkload

			Eventually(func(g Gomega) {
				var taskWorkloads korifiv1alpha1.TaskWorkloadList

				g.Expect(k8sClient.List(ctx, &taskWorkloads,
					client.InNamespace(cfSpace.Status.GUID),
					client.MatchingLabels{korifiv1alpha1.CFTaskGUIDLabelKey: cfTask.Name},
				)).To(Succeed())
				g.Expect(taskWorkloads.Items).To(HaveLen(1))

				taskWorkload = taskWorkloads.Items[0]
				g.Expect(taskWorkload.Name).To(Equal(cfTask.Name))
				g.Expect(taskWorkload.Spec.Command).To(Equal([]string{"/cnb/lifecycle/launcher", "echo hello"}))
				g.Expect(taskWorkload.Spec.Image).To(Equal("registry.io/my/image"))
				g.Expect(taskWorkload.Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{{Name: "registry-secret"}}))
				g.Expect(taskWorkload.Spec.Resources.Requests.Memory().String()).To(Equal(fmt.Sprintf("%dM", defaultMemoryMB)))
				g.Expect(taskWorkload.Spec.Resources.Limits.Memory().String()).To(Equal(fmt.Sprintf("%dM", defaultMemoryMB)))
				g.Expect(taskWorkload.Spec.Resources.Requests.StorageEphemeral().String()).To(Equal(fmt.Sprintf("%dM", defaultDiskQuotaMB)))
				g.Expect(taskWorkload.Spec.Resources.Limits.StorageEphemeral().String()).To(Equal(fmt.Sprintf("%dM", defaultDiskQuotaMB)))
				g.Expect(taskWorkload.Spec.Resources.Requests.Cpu().String()).To(Equal("75m"))
				g.Expect(taskWorkload.GetOwnerReferences()).To(ConsistOf(SatisfyAll(
					HaveField("Name", cfTask.Name),
					HaveField("Controller", PointTo(BeTrue())),
				)))
			}).Should(Succeed())

			// Refresh the VCAPServicesSecretName
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())

			Expect(taskWorkload.Spec.Env).To(ConsistOf(
				corev1.EnvVar{
					Name: "BOB",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: envSecret.Name,
							},
							Key: "BOB",
						},
					},
				},
				corev1.EnvVar{
					Name: "FAST",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: envSecret.Name,
							},
							Key: "FAST",
						},
					},
				},
				corev1.EnvVar{
					Name: "VCAP_SERVICES",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Status.VCAPServicesSecretName,
							},
							Key: "VCAP_SERVICES",
						},
					},
				},
				corev1.EnvVar{
					Name: "VCAP_APPLICATION",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Status.VCAPApplicationSecretName,
							},
							Key: "VCAP_APPLICATION",
						},
					},
				},
			))
		})

		It("records a TaskWorkloadCreated event", func() {
			Expect(eventRecorder.EventfCallCount()).To(Equal(eventCallCount+1), "eventRecorder.Eventf call count mismatch")
			eventTaskObj, eventType, eventReason, eventMessage, eventMessageArgs := eventRecorder.EventfArgsForCall(eventCallCount)
			eventTask := eventTaskObj.(*korifiv1alpha1.CFTask)
			Expect(client.ObjectKeyFromObject(eventTask)).To(Equal(client.ObjectKeyFromObject(task)))
			Expect(eventType).To(Equal("Normal"), "Unexpected event type in event record")
			Expect(eventReason).To(Equal("TaskWorkloadCreated"), "Unexpected event reason in event record")
			Expect(eventMessage).To(Equal("Created task workload %s"), "Unexpected event message in event record")
			Expect(eventMessageArgs).To(Equal([]interface{}{task.Name}), "Unexpected event message args in event record")
		})

		When("the task workload status condition changes", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					var taskWorkloads korifiv1alpha1.TaskWorkloadList

					g.Expect(k8sClient.List(ctx, &taskWorkloads,
						client.InNamespace(cfSpace.Status.GUID),
						client.MatchingLabels{korifiv1alpha1.CFTaskGUIDLabelKey: cfTask.Name},
					)).To(Succeed())
					g.Expect(taskWorkloads.Items).To(HaveLen(1))

					modifiedTaskWorkload := taskWorkloads.Items[0].DeepCopy()
					g.Expect(k8s.Patch(ctx, k8sClient, modifiedTaskWorkload, func() {
						meta.SetStatusCondition(&modifiedTaskWorkload.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.TaskStartedConditionType,
							Status:  metav1.ConditionTrue,
							Reason:  "task_started",
							Message: "task started",
						})
					})).To(Succeed())
				}).Should(Succeed())
			})

			It("reflects the status in the korifi task", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Status.GUID, Name: cfTask.Name}, task)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskStartedConditionType)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	Describe("CFTask Cancellation", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfTask)).To(Succeed())
		})

		When("spec.canceled is set to true", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfTask, func() {
					cfTask.Spec.Canceled = true
				})).To(Succeed())
			})

			It("sets the canceled status condition", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfTask), cfTask)).To(Succeed())
					canceledStatusCondition := meta.FindStatusCondition(cfTask.Status.Conditions, korifiv1alpha1.TaskCanceledConditionType)
					g.Expect(canceledStatusCondition).NotTo(BeNil())
					g.Expect(canceledStatusCondition.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(canceledStatusCondition.Reason).To(Equal("TaskCanceled"))
					g.Expect(canceledStatusCondition.ObservedGeneration).To(Equal(cfTask.Generation))
				}).Should(Succeed())
			})
		})
	})

	Describe("CFTask TTL", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfTask)).To(Succeed())

			updatedTask := cfTask.DeepCopy()
			meta.SetStatusCondition(&updatedTask.Status.Conditions, metav1.Condition{
				Type:               korifiv1alpha1.TaskSucceededConditionType,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "succeeded",
				Message:            "succeeded",
			})
			Expect(k8sClient.Status().Patch(ctx, updatedTask, client.MergeFrom(cfTask))).To(Succeed())
		})

		It("it can get the task shortly after completion", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfTask), cfTask)).To(Succeed())
		})

		It("deletes the task after it expires", func() {
			task := new(korifiv1alpha1.CFTask)

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfTask), task)
				g.Expect(err).To(HaveOccurred(), "Task has not been deleted")
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
