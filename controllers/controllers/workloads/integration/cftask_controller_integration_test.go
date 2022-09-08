package integration_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"
)

var _ = Describe("CFTaskReconciler Integration Tests", func() {
	var (
		ctx context.Context
		ns  string

		cfTask    *korifiv1alpha1.CFTask
		cfApp     *korifiv1alpha1.CFApp
		cfDroplet *korifiv1alpha1.CFBuild
		envSecret *corev1.Secret
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = testutils.PrefixedGUID("namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		})).To(Succeed())

		cfDroplet = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      testutils.PrefixedGUID("droplet"),
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				Lifecycle: korifiv1alpha1.Lifecycle{Type: "buildpack"},
			},
		}
		Expect(k8sClient.Create(ctx, cfDroplet)).To(Succeed())

		Expect(k8s.PatchStatus(context.Background(), k8sClient, cfDroplet, func() {
			cfDroplet.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
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
		}, metav1.Condition{
			Type:   "type",
			Status: "Unknown",
			Reason: "reason",
		})).To(Succeed())

		envSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.PrefixedGUID("env-secret"),
				Namespace: ns,
			},
			StringData: map[string]string{
				"BOB":  "flemming",
				"FAST": "show",
			},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(k8sClient.Create(ctx, envSecret)).To(Succeed())

		cfAppName := testutils.PrefixedGUID("app")

		cfProcess := &korifiv1alpha1.CFProcess{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      testutils.PrefixedGUID("web-process"),
				Labels: map[string]string{
					korifiv1alpha1.CFProcessTypeLabelKey: "web",
					korifiv1alpha1.CFAppGUIDLabelKey:     cfAppName,
				},
			},
			Spec: korifiv1alpha1.CFProcessSpec{
				AppRef:      corev1.LocalObjectReference{Name: cfAppName},
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
				Namespace: ns,
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
				Namespace: ns,
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
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfTask)).To(Succeed())
		})

		When("the task gets initialized", func() {
			var task *korifiv1alpha1.CFTask

			BeforeEach(func() {
				task = &korifiv1alpha1.CFTask{}
			})

			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cfTask.Name}, task)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue(), "task did not become initialized")
				}).Should(Succeed())
			})

			It("populates the Status of the CFTask", func() {
				Expect(task.Status.SequenceID).NotTo(BeZero())
				Expect(task.Status.MemoryMB).To(Equal(cfProcessDefaults.MemoryMB))
				Expect(task.Status.DiskQuotaMB).To(Equal(cfProcessDefaults.DiskQuotaMB))
				Expect(task.Status.DropletRef.Name).To(Equal(cfDroplet.Name))
			})
		})

		It("SequenceID does not change on task update", func() {
			var task korifiv1alpha1.CFTask

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cfTask.Name}, &task)).To(Succeed())
				g.Expect(task.Status.SequenceID).NotTo(BeZero())
			}).Should(Succeed())

			seqId := task.Status.SequenceID

			updatedTask := task.DeepCopy()
			updatedTask.Spec.Command = "foo bar"
			Expect(k8sClient.Patch(ctx, updatedTask, client.MergeFrom(&task))).To(Succeed())

			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cfTask.Name}, &task)).To(Succeed())
				g.Expect(task.Status.SequenceID).To(Equal(seqId))
			}).Should(Succeed())
		})

		It("creates an TaskWorkload", func() {
			var taskWorkloads korifiv1alpha1.TaskWorkloadList
			Eventually(func() ([]korifiv1alpha1.TaskWorkload, error) {
				err := k8sClient.List(
					ctx,
					&taskWorkloads,
					client.InNamespace(ns),
					client.MatchingLabels{korifiv1alpha1.CFTaskGUIDLabelKey: cfTask.Name},
				)
				return taskWorkloads.Items, err
			}).Should(HaveLen(1))

			Expect(taskWorkloads.Items[0].Name).To(Equal(cfTask.Name))
			Expect(taskWorkloads.Items[0].Spec.Command).To(Equal([]string{"/cnb/lifecycle/launcher", "echo hello"}))
			Expect(taskWorkloads.Items[0].Spec.Image).To(Equal("registry.io/my/image"))
			Expect(taskWorkloads.Items[0].Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{{Name: "registry-secret"}}))
			Expect(taskWorkloads.Items[0].Spec.Resources.Requests.Memory().String()).To(Equal(fmt.Sprintf("%dM", cfProcessDefaults.MemoryMB)))
			Expect(taskWorkloads.Items[0].Spec.Resources.Limits.Memory().String()).To(Equal(fmt.Sprintf("%dM", cfProcessDefaults.MemoryMB)))
			Expect(taskWorkloads.Items[0].Spec.Resources.Requests.StorageEphemeral().String()).To(Equal(fmt.Sprintf("%dM", cfProcessDefaults.DiskQuotaMB)))
			Expect(taskWorkloads.Items[0].Spec.Resources.Limits.StorageEphemeral().String()).To(Equal(fmt.Sprintf("%dM", cfProcessDefaults.DiskQuotaMB)))
			Expect(taskWorkloads.Items[0].Spec.Resources.Requests.Cpu().String()).To(Equal("75m"))

			// refresh the VCAPServicesSecretName
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())

			Expect(taskWorkloads.Items[0].Spec.Env).To(ConsistOf(
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
			))
		})

		When("the task workload status condition changes", func() {
			JustBeforeEach(func() {
				var taskWorkloads korifiv1alpha1.TaskWorkloadList
				Eventually(func() ([]korifiv1alpha1.TaskWorkload, error) {
					err := k8sClient.List(
						ctx,
						&taskWorkloads,
						client.InNamespace(ns),
						client.MatchingLabels{korifiv1alpha1.CFTaskGUIDLabelKey: cfTask.Name},
					)
					return taskWorkloads.Items, err
				}).Should(HaveLen(1))

				modifiedTaskWorkload := taskWorkloads.Items[0].DeepCopy()
				meta.SetStatusCondition(&modifiedTaskWorkload.Status.Conditions, metav1.Condition{
					Type:    korifiv1alpha1.TaskStartedConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  "task_started",
					Message: "task started",
				})

				Expect(k8sClient.Status().Patch(ctx, modifiedTaskWorkload, client.MergeFrom(&taskWorkloads.Items[0]))).To(Succeed())
			})

			It("reflects the status in the korifi task", func() {
				Eventually(func(g Gomega) {
					var task korifiv1alpha1.CFTask

					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cfTask.Name}, &task)).To(Succeed())
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
				cfTask.Spec.Canceled = true
				Expect(k8sClient.Update(ctx, cfTask)).To(Succeed())
			})

			It("sets the canceled status condition", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfTask), cfTask)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(cfTask.Status.Conditions, korifiv1alpha1.TaskCanceledConditionType)).To(BeTrue())
				})
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
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfTask), cfTask)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
