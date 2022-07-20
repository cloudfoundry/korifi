package repositories_test

import (
	"context"
	"sync"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
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
		taskRepo            *repositories.TaskRepo
		org                 *korifiv1alpha1.CFOrg
		space               *korifiv1alpha1.CFSpace
		cfApp               *korifiv1alpha1.CFApp
		dummyTaskController func(*korifiv1alpha1.CFTask)
		controllerSync      *sync.WaitGroup
		killController      chan bool
	)

	setStatusAndUpdate := func(task *korifiv1alpha1.CFTask, conditionTypes ...string) {
		if task.Status.Conditions == nil {
			task.Status.Conditions = []metav1.Condition{}
		}

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

	defaultStatusValues := func(task *korifiv1alpha1.CFTask, seqId int64, dropletId string) *korifiv1alpha1.CFTask {
		task.Status.SequenceID = seqId
		task.Status.MemoryMB = 256
		task.Status.DiskQuotaMB = 128
		task.Status.DropletRef.Name = dropletId

		return task
	}

	BeforeEach(func() {
		taskRepo = repositories.NewTaskRepo(userClientFactory, namespaceRetriever, nsPerms, conditions.NewConditionAwaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList](2*time.Second))

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		cfApp = createApp(space.Name)

		dummyTaskController = func(cft *korifiv1alpha1.CFTask) {}
		killController = make(chan bool)
	})

	JustBeforeEach(func() {
		tasksWatch, err := k8sClient.Watch(
			ctx,
			&korifiv1alpha1.CFTaskList{},
			client.InNamespace(space.Name),
		)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			tasksWatch.Stop()
		})
		watchChan := tasksWatch.ResultChan()

		controllerSync = &sync.WaitGroup{}
		controllerSync.Add(1)

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
				case <-timer.C:
					return

				case <-killController:
					return
				}
			}
		}()
	})

	AfterEach(func() {
		close(killController)
		controllerSync.Wait()
	})

	Describe("CreateTask", func() {
		var (
			createMessage repositories.CreateTaskMessage
			taskRecord    repositories.TaskRecord
			createErr     error
		)

		BeforeEach(func() {
			dummyTaskController = func(cft *korifiv1alpha1.CFTask) {
				setStatusAndUpdate(
					defaultStatusValues(cft, 6, cfApp.Spec.CurrentDropletRef.Name),
					korifiv1alpha1.TaskInitializedConditionType,
				)
			}
			createMessage = repositories.CreateTaskMessage{
				Command:   "  echo    hello  ",
				SpaceGUID: space.Name,
				AppGUID:   cfApp.Name,
			}
		})

		JustBeforeEach(func() {
			taskRecord, createErr = taskRepo.CreateTask(ctx, authInfo, createMessage)
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
					Expect(createErr).To(MatchError(ContainSubstring("did not get the Initialized condition")))
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

			setStatusAndUpdate(defaultStatusValues(cfTask, 6, cfApp.Spec.CurrentDropletRef.Name))
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
					setStatusAndUpdate(
						defaultStatusValues(cfTask, 6, cfApp.Spec.CurrentDropletRef.Name),
						korifiv1alpha1.TaskInitializedConditionType,
					)
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
					Expect(taskRecord.FailureReason).To(Equal("bar"))
				})
			})

			When("the task was canceled", func() {
				BeforeEach(func() {
					setStatusAndUpdate(cfTask, korifiv1alpha1.TaskInitializedConditionType, korifiv1alpha1.TaskStartedConditionType)
					meta.SetStatusCondition(&(cfTask.Status.Conditions), metav1.Condition{
						Type:   korifiv1alpha1.TaskFailedConditionType,
						Status: metav1.ConditionTrue,
						Reason: "taskCanceled",
					})
					Expect(k8sClient.Status().Update(ctx, cfTask)).To(Succeed())
				})

				It("returns the failed task", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(taskRecord.State).To(Equal(repositories.TaskStateFailed))
					Expect(taskRecord.FailureReason).To(Equal("task was canceled"))
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

	Describe("List Tasks", func() {
		var (
			cfApp2      *korifiv1alpha1.CFApp
			task1       *korifiv1alpha1.CFTask
			task2       *korifiv1alpha1.CFTask
			space2      *korifiv1alpha1.CFSpace
			listTaskMsg repositories.ListTaskMessage

			listedTasks []repositories.TaskRecord
			listErr     error
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))
			cfApp2 = createApp(space2.Name)
			listTaskMsg = repositories.ListTaskMessage{}

			task1 = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prefixedGUID("task1"),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: []string{"echo", "hello"},
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), task1)).To(Succeed())

			task2 = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prefixedGUID("task2"),
					Namespace: space2.Name,
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: []string{"echo", "hello"},
					AppRef: corev1.LocalObjectReference{
						Name: cfApp2.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), task2)).To(Succeed())
		})

		JustBeforeEach(func() {
			listedTasks, listErr = taskRepo.ListTasks(ctx, authInfo, listTaskMsg)
		})

		It("returs an empty list due to no permissions", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedTasks).To(BeEmpty())
		})

		When("the user has the space developer role in space2", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("lists tasks from that namespace only", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedTasks).To(HaveLen(1))
				Expect(listedTasks[0].Name).To(Equal(task2.Name))
			})

			When("the user has a useless binding in space1", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, rootNamespaceUserRole.Name, space.Name)
				})

				It("still lists tasks from that namespace only", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(listedTasks).To(HaveLen(1))
					Expect(listedTasks[0].Name).To(Equal(task2.Name))
				})
			})

			When("filtering tasks by apps with permissions for both", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				})

				When("the app1 guid is passed as a filter", func() {
					BeforeEach(func() {
						listTaskMsg.AppGUIDs = []string{cfApp.Name}
					})

					It("lists tasks for that app", func() {
						Expect(listErr).NotTo(HaveOccurred())
						Expect(listedTasks).To(HaveLen(1))
						Expect(listedTasks[0].Name).To(Equal(task1.Name))
					})
				})

				When("the app2 guid is passed as a filter", func() {
					BeforeEach(func() {
						listTaskMsg.AppGUIDs = []string{cfApp2.Name}
					})

					It("lists tasks for that app", func() {
						Expect(listErr).NotTo(HaveOccurred())
						Expect(listedTasks).To(HaveLen(1))
						Expect(listedTasks[0].Name).To(Equal(task2.Name))
					})
				})

				When("app guid and sequence IDs are passed as a filter", func() {
					BeforeEach(func() {
						setStatusAndUpdate(
							defaultStatusValues(task2, 2, cfApp2.Spec.CurrentDropletRef.Name),
							korifiv1alpha1.TaskInitializedConditionType,
						)

						task21 := &korifiv1alpha1.CFTask{
							ObjectMeta: metav1.ObjectMeta{
								Name:      prefixedGUID("task21"),
								Namespace: space2.Name,
							},
							Spec: korifiv1alpha1.CFTaskSpec{
								Command: []string{"echo", "hello"},
								AppRef: corev1.LocalObjectReference{
									Name: cfApp2.Name,
								},
							},
						}
						Expect(k8sClient.Create(context.Background(), task21)).To(Succeed())
						setStatusAndUpdate(
							defaultStatusValues(task21, 21, cfApp2.Spec.CurrentDropletRef.Name),
							korifiv1alpha1.TaskInitializedConditionType,
						)

						listTaskMsg.AppGUIDs = []string{cfApp2.Name}
						listTaskMsg.SequenceIDs = []int64{2}
					})

					It("returns the tasks filtered by sequence ID", func() {
						Expect(listErr).NotTo(HaveOccurred())
						Expect(listedTasks).To(HaveLen(1))
						Expect(listedTasks[0].Name).To(Equal(task2.Name))
					})
				})

				When("filtering by a non-existant app guid", func() {
					BeforeEach(func() {
						listTaskMsg.AppGUIDs = []string{"does-not-exist"}
					})

					It("returns an empty list", func() {
						Expect(listErr).NotTo(HaveOccurred())
						Expect(listedTasks).To(BeEmpty())
					})
				})
			})
		})
	})

	Describe("Cancel Task", func() {
		var (
			taskGUID   string
			cancelErr  error
			taskRecord repositories.TaskRecord
		)

		BeforeEach(func() {
			dummyTaskController = func(cft *korifiv1alpha1.CFTask) {
				if cft.Spec.Canceled {
					setStatusAndUpdate(cft, korifiv1alpha1.TaskCanceledConditionType)
				}
			}

			taskGUID = generateGUID()
			cfTask := &korifiv1alpha1.CFTask{
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

			cfTask.Status.SequenceID = 6
			cfTask.Status.MemoryMB = 256
			cfTask.Status.DiskQuotaMB = 128
			cfTask.Status.DropletRef.Name = cfApp.Spec.CurrentDropletRef.Name
			setStatusAndUpdate(cfTask, korifiv1alpha1.TaskInitializedConditionType, korifiv1alpha1.TaskStartedConditionType)
		})

		JustBeforeEach(func() {
			taskRecord, cancelErr = taskRepo.CancelTask(ctx, authInfo, taskGUID)
		})

		It("returns forbidden", func() {
			Expect(cancelErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("cancels the task", func() {
				Expect(cancelErr).NotTo(HaveOccurred())
				Expect(taskRecord.Name).To(Equal(taskGUID))
				Expect(taskRecord.GUID).NotTo(BeEmpty())
				Expect(taskRecord.Command).To(Equal("echo hello"))
				Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(taskRecord.SequenceID).To(BeEquivalentTo(6))
				Expect(taskRecord.CreationTimestamp).To(BeTemporally("~", time.Now(), 5*time.Second))
				Expect(taskRecord.MemoryMB).To(BeEquivalentTo(256))
				Expect(taskRecord.DiskMB).To(BeEquivalentTo(128))
				Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(taskRecord.State).To(Equal(repositories.TaskStateCanceling))
			})

			When("the status is not updated within the timeout", func() {
				BeforeEach(func() {
					dummyTaskController = func(*korifiv1alpha1.CFTask) {}
				})

				It("returns a timeout error", func() {
					Expect(cancelErr).To(MatchError(ContainSubstring("did not get the Canceled condition")))
				})
			})
		})
	})
})
