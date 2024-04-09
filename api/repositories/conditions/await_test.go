package conditions_test

import (
	"errors"
	"sync"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("StateAwaiter", func() {
	var (
		awaiter     *conditions.Awaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList, *korifiv1alpha1.CFTaskList]
		task        *korifiv1alpha1.CFTask
		awaitedTask *korifiv1alpha1.CFTask
		awaitErr    error
	)

	asyncPatchTask := func(patchTask func(*korifiv1alpha1.CFTask)) {
		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer GinkgoRecover()
			defer wg.Done()

			patchedTask := task.DeepCopy()
			Expect(k8s.Patch(ctx, k8sClient, patchedTask, func() {
				patchTask(patchedTask)
			})).To(Succeed())
		}()

		DeferCleanup(func() {
			wg.Wait()
		})
	}

	BeforeEach(func() {
		awaiter = conditions.NewStateAwaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList](time.Second)
		awaitedTask = nil
		awaitErr = nil

		task = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-task",
			},
		}

		Expect(k8sClient.Create(ctx, task)).To(Succeed())
	})

	Describe("AwaitState", func() {
		JustBeforeEach(func() {
			awaitedTask, awaitErr = awaiter.AwaitState(ctx, k8sClient, task, func(actualTask *korifiv1alpha1.CFTask) error {
				if actualTask.Status.DropletRef.Name == "" {
					return errors.New("droplet ref not set")
				}

				return nil
			})
		})

		It("returns an error as the desired state is never reached", func() {
			Expect(awaitErr).To(MatchError(ContainSubstring("droplet ref not set")))
		})

		When("the desired state is reached", func() {
			BeforeEach(func() {
				asyncPatchTask(func(cfTask *korifiv1alpha1.CFTask) {
					cfTask.Status.DropletRef.Name = "some-droplet"
				})
			})

			It("succeeds and returns the updated object", func() {
				Expect(awaitErr).NotTo(HaveOccurred())
				Expect(awaitedTask).NotTo(BeNil())

				Expect(awaitedTask.Name).To(Equal(task.Name))
				Expect(awaitedTask.Status.DropletRef.Name).To(Equal("some-droplet"))
			})
		})
	})

	Describe("AwaitCondition", func() {
		JustBeforeEach(func() {
			awaitedTask, awaitErr = awaiter.AwaitCondition(ctx, k8sClient, task, korifiv1alpha1.TaskInitializedConditionType)
		})

		It("returns an error as the condition never becomes true", func() {
			Expect(awaitErr).To(MatchError(ContainSubstring("expected the Initialized condition to be true")))
		})

		When("the condition becomes true", func() {
			BeforeEach(func() {
				asyncPatchTask(func(cfTask *korifiv1alpha1.CFTask) {
					meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.TaskInitializedConditionType,
						Status: metav1.ConditionTrue,
						Reason: "initialized",
					})
				})
			})

			It("succeeds and returns the updated object", func() {
				Expect(awaitErr).NotTo(HaveOccurred())
				Expect(awaitedTask).NotTo(BeNil())

				Expect(awaitedTask.Name).To(Equal(task.Name))
				Expect(meta.IsStatusConditionTrue(awaitedTask.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue())
			})
		})
	})
})
