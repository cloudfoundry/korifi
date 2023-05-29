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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Await", func() {
	var (
		awaiter     *conditions.Awaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList, *korifiv1alpha1.CFTaskList]
		task        *korifiv1alpha1.CFTask
		awaitedTask *korifiv1alpha1.CFTask
		awaitErr    error
	)

	BeforeEach(func() {
		awaiter = conditions.NewConditionAwaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList](time.Second)
		awaitedTask = nil
		awaitErr = nil

		task = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-task",
			},
		}

		Expect(k8sClient.Create(context.Background(), task)).To(Succeed())
	})

	JustBeforeEach(func() {
		awaitedTask, awaitErr = awaiter.AwaitCondition(context.Background(), k8sClient, task, korifiv1alpha1.TaskInitializedConditionType)
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

				taskCopy := task.DeepCopy()
				meta.SetStatusCondition(&taskCopy.Status.Conditions, metav1.Condition{
					Type:   korifiv1alpha1.TaskInitializedConditionType,
					Status: metav1.ConditionTrue,
					Reason: "initialized",
				})

				Expect(k8sClient.Status().Patch(context.Background(), taskCopy, client.MergeFrom(task))).To(Succeed())
			}()
		})

		AfterEach(func() {
			wg.Wait()
		})

		It("succeeds and returns the updated object", func() {
			Expect(awaitErr).NotTo(HaveOccurred())
			Expect(awaitedTask).NotTo(BeNil())

			Expect(awaitedTask.Name).To(Equal(task.Name))
			Expect(meta.IsStatusConditionTrue(awaitedTask.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue())
		})
	})
})
