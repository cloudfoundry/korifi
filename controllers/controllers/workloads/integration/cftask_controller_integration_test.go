package integration_test

import (
	"context"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
)

var _ = Describe("CFTaskReconciler Integration Tests", func() {
	var (
		ctx context.Context
		ns  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = testutils.PrefixedGUID("namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		})).To(Succeed())
	})

	Describe("CFTask creation", func() {
		var cfTask *korifiv1alpha1.CFTask
		var cfApp *korifiv1alpha1.CFApp
		var cfDroplet *korifiv1alpha1.CFBuild

		BeforeEach(func() {
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

			cfDropletCopy := cfDroplet.DeepCopy()
			cfDropletCopy.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
				Registry: korifiv1alpha1.Registry{Image: "registry.io/my/image"},
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

			cfApp = &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      testutils.PrefixedGUID("app"),
				},
				Spec: korifiv1alpha1.CFAppSpec{
					Lifecycle: korifiv1alpha1.Lifecycle{Type: "buildpack"},
					CurrentDropletRef: corev1.LocalObjectReference{
						Name: cfDroplet.Name,
					},
					DesiredState: "STOPPED",
					DisplayName:  "app",
				},
			}
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

			cfTask = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      testutils.PrefixedGUID("cftask"),
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: []string{"echo", "hello"},
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
		})

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
			updatedTask.Spec.Command = []string{"foo", "bar"}
			Expect(k8sClient.Patch(ctx, updatedTask, client.MergeFrom(&task))).To(Succeed())

			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cfTask.Name}, &task)).To(Succeed())
				g.Expect(task.Status.SequenceID).To(Equal(seqId))
			}).Should(Succeed())
		})

		It("creates an eirini.Task", func() {
			var tasks eiriniv1.TaskList
			Eventually(func() ([]eiriniv1.Task, error) {
				err := k8sClient.List(
					ctx,
					&tasks,
					client.InNamespace(ns),
					client.MatchingLabels{korifiv1alpha1.CFTaskGUIDLabelKey: cfTask.Name},
				)
				return tasks.Items, err
			}).Should(HaveLen(1))

			Expect(tasks.Items[0].Name).To(Equal(cfTask.Name))
			Expect(tasks.Items[0].Spec.GUID).To(Equal(cfTask.Name))
			Expect(tasks.Items[0].Spec.Command).To(ConsistOf("echo", "hello"))
			Expect(tasks.Items[0].Spec.Image).To(Equal("registry.io/my/image"))
			Expect(tasks.Items[0].Spec.MemoryMB).To(Equal(cfProcessDefaults.MemoryMB))
			Expect(tasks.Items[0].Spec.DiskMB).To(Equal(cfProcessDefaults.DiskQuotaMB))
		})
	})
})
