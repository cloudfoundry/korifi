package repositories_test

import (
	"context"
	"sync"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TaskRepository", func() {
	var (
		taskRepo *repositories.TaskRepo
		org      *korifiv1alpha1.CFOrg
		space    *korifiv1alpha1.CFSpace
		cfApp    *korifiv1alpha1.CFApp
	)

	setStatusAndUpdate := func(task *korifiv1alpha1.CFTask, conditionTypes ...string) {
		for _, cond := range conditionTypes {
			meta.SetStatusCondition(&(task.Status.Conditions), metav1.Condition{
				Type:    cond,
				Status:  metav1.ConditionTrue,
				Reason:  "foo",
				Message: "bar",
			})
		}
		ExpectWithOffset(1, k8sClient.Status().Update(ctx, task)).To(Succeed())
	}

	BeforeEach(func() {
		taskRepo = repositories.NewTaskRepo(userClientFactory, namespaceRetriever, 2*time.Second)

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		cfApp = createApp(space.Name)
	})

	Describe("CreateTask", func() {
		var (
			createMessage       repositories.CreateTaskMessage
			taskRecord          repositories.TaskRecord
			createErr           error
			dummyTaskController func(*korifiv1alpha1.CFTask)
			killController      chan bool
			controllerSync      *sync.WaitGroup
		)

		BeforeEach(func() {
			dummyTaskController = func(cft *korifiv1alpha1.CFTask) {
				cft.Status.SequenceID = 6
				cft.Status.MemoryMB = 256
				cft.Status.DiskQuotaMB = 128
				cft.Status.DropletRef.Name = cfApp.Spec.CurrentDropletRef.Name
				setStatusAndUpdate(cft, korifiv1alpha1.TaskInitializedConditionType)
			}
			controllerSync = &sync.WaitGroup{}
			controllerSync.Add(1)
			killController = make(chan bool)
			createMessage = repositories.CreateTaskMessage{
				Command:   "  echo    hello  ",
				SpaceGUID: space.Name,
				AppGUID:   cfApp.Name,
			}
		})

		JustBeforeEach(func() {
			tasksWatch, err := k8sClient.Watch(
				ctx,
				&korifiv1alpha1.CFTaskList{},
				client.InNamespace(space.Name),
			)
			Expect(err).NotTo(HaveOccurred())

			defer tasksWatch.Stop()
			watchChan := tasksWatch.ResultChan()

			go func() {
				defer GinkgoRecover()
				defer controllerSync.Done()

				timer := time.NewTimer(2 * time.Second)
				defer timer.Stop()

				for {
					select {
					case e := <-watchChan:
						cft, ok := e.Object.(*korifiv1alpha1.CFTask)
						if !ok {
							time.Sleep(100 * time.Millisecond)
							continue
						}

						dummyTaskController(cft)
						return

					case <-timer.C:
						return

					case <-killController:
						return
					}
				}
			}()

			taskRecord, createErr = taskRepo.CreateTask(ctx, authInfo, createMessage)
		})

		AfterEach(func() {
			close(killController)
			controllerSync.Wait()
		})

		It("returns forbidden error", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can create tasks", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates the task", func() {
				Expect(createErr).NotTo(HaveOccurred())
				Expect(taskRecord.Name).NotTo(BeEmpty())
				Expect(taskRecord.GUID).NotTo(BeEmpty())
				Expect(taskRecord.Command).To(Equal("echo hello"))
				Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(taskRecord.SequenceID).NotTo(BeZero())
				Expect(taskRecord.CreationTimestamp).To(BeTemporally("~", time.Now(), 5*time.Second))
				Expect(taskRecord.MemoryMB).To(BeEquivalentTo(256))
				Expect(taskRecord.DiskMB).To(BeEquivalentTo(128))
				Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(taskRecord.State).To(Equal(repositories.TaskStatePending))
			})

			When("the task never becomes initialized", func() {
				BeforeEach(func() {
					dummyTaskController = func(cft *korifiv1alpha1.CFTask) {}
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("did not get initialized")))
				})
			})
		})

		When("unprivileged client creation fails", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("failed to build user client")))
			})
		})
	})

	Describe("GetTask", func() {
		var (
			taskRecord repositories.TaskRecord
			getErr     error
			taskGUID   string
			cfTask     *korifiv1alpha1.CFTask
		)

		BeforeEach(func() {
			taskGUID = generateGUID()
			cfTask = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: []string{"echo", "hello"},
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfTask)).To(Succeed())

			cfTask.Status.Conditions = []metav1.Condition{}
			cfTask.Status.SequenceID = 6
			cfTask.Status.MemoryMB = 256
			cfTask.Status.DiskQuotaMB = 128
			cfTask.Status.DropletRef.Name = cfApp.Spec.CurrentDropletRef.Name
			setStatusAndUpdate(cfTask)
		})

		JustBeforeEach(func() {
			taskRecord, getErr = taskRepo.GetTask(ctx, authInfo, taskGUID)
		})

		It("returns a forbidden error", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can get tasks", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("the task has not been initialized yet", func() {
				It("returns a not found error", func() {
					Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("the task is ready", func() {
				BeforeEach(func() {
					setStatusAndUpdate(cfTask, korifiv1alpha1.TaskInitializedConditionType)
				})

				It("returns the task", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(taskRecord.Name).To(Equal(taskGUID))
					Expect(taskRecord.GUID).NotTo(BeEmpty())
					Expect(taskRecord.Command).To(Equal("echo hello"))
					Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
					Expect(taskRecord.SequenceID).To(BeEquivalentTo(6))
					Expect(taskRecord.CreationTimestamp).To(BeTemporally("~", time.Now(), 5*time.Second))
					Expect(taskRecord.MemoryMB).To(BeEquivalentTo(256))
					Expect(taskRecord.DiskMB).To(BeEquivalentTo(128))
					Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
					Expect(taskRecord.State).To(Equal(repositories.TaskStatePending))
				})
			})

			When("the task is running", func() {
				BeforeEach(func() {
					setStatusAndUpdate(cfTask, korifiv1alpha1.TaskInitializedConditionType, korifiv1alpha1.TaskStartedConditionType)
				})

				It("returns the running task", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(taskRecord.State).To(Equal(repositories.TaskStateRunning))
				})
			})

			When("the task has succeeded", func() {
				BeforeEach(func() {
					setStatusAndUpdate(cfTask, korifiv1alpha1.TaskInitializedConditionType, korifiv1alpha1.TaskStartedConditionType, korifiv1alpha1.TaskSucceededConditionType)
				})

				It("returns the succeeded task", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(taskRecord.State).To(Equal(repositories.TaskStateSucceeded))
				})
			})

			When("the task has failed", func() {
				BeforeEach(func() {
					setStatusAndUpdate(cfTask, korifiv1alpha1.TaskInitializedConditionType, korifiv1alpha1.TaskStartedConditionType, korifiv1alpha1.TaskFailedConditionType)
				})

				It("returns the failed task", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(taskRecord.State).To(Equal(repositories.TaskStateFailed))
				})
			})
		})

		When("the task doesn't exist", func() {
			BeforeEach(func() {
				taskGUID = "does-not-exist"
			})

			It("returns a not found error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("unprivileged client creation fails", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("returns an error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("failed to build user client")))
			})
		})
	})
})
