package conditions_test

import (
	"context"
	"sync"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Await", func() {
	var (
		awaiter *conditions.Awaiter
		task    *korifiv1alpha1.CFTask

		awaitedObject runtime.Object
		awaitErr      error
	)

	BeforeEach(func() {
		awaiter = conditions.NewCFTaskConditionAwaiter(time.Second)
		awaitedObject = nil
		awaitErr = nil

		task = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-task",
			},
			Spec: korifiv1alpha1.CFTaskSpec{},
		}

		Expect(k8sClient.Create(context.Background(), task)).To(Succeed())
	})

	JustBeforeEach(func() {
		awaitedObject, awaitErr = awaiter.AwaitCondition(context.Background(), k8sClient, task, korifiv1alpha1.TaskInitializedConditionType)
	})

	It("returns an error as the condition never becomes true", func() {
		Expect(awaitErr).To(MatchError(ContainSubstring("did not get the Initialized condition")))
	})

	When("the condition becomes true", func() {
		var wg sync.WaitGroup

		BeforeEach(func() {
			wg.Add(1)

			go func() {
				defer GinkgoRecover()
				defer wg.Done()

				time.Sleep(500 * time.Millisecond)
				taskCopy := task.DeepCopy()
				taskCopy.Status = korifiv1alpha1.CFTaskStatus{
					Conditions: []metav1.Condition{{
						Type:               korifiv1alpha1.TaskInitializedConditionType,
						Status:             metav1.ConditionTrue,
						Reason:             "initialized",
						Message:            "initialized",
						LastTransitionTime: metav1.Now(),
					}},
				}

				Expect(k8sClient.Status().Patch(context.Background(), taskCopy, client.MergeFrom(task))).To(Succeed())
			}()
		})

		AfterEach(func() {
			wg.Wait()
		})

		It("succeeds and returns the updated object", func() {
			Eventually(func(g Gomega) {
				g.Expect(awaitErr).NotTo(HaveOccurred())
				g.Expect(awaitedObject).NotTo(BeNil())

				awaitedTask, ok := awaitedObject.(*korifiv1alpha1.CFTask)
				g.Expect(ok).To(BeTrue())
				g.Expect(awaitedTask.Name).To(Equal(task.Name))
				g.Expect(meta.IsStatusConditionTrue(awaitedTask.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
