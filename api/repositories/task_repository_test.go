package repositories_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TaskRepository", func() {
	var (
		conditionAwaiter *fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFTask,
			korifiv1alpha1.CFTaskList,
			*korifiv1alpha1.CFTaskList,
		]
		taskRepo *repositories.TaskRepo
		org      *korifiv1alpha1.CFOrg
		space    *korifiv1alpha1.CFSpace
		cfApp    *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		conditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFTask,
			korifiv1alpha1.CFTaskList,
			*korifiv1alpha1.CFTaskList,
		]{}
		taskRepo = repositories.NewTaskRepo(userClientFactory, namespaceRetriever, nsPerms, conditionAwaiter)

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		cfApp = createApp(space.Name)
	})

	Describe("CreateTask", func() {
		var (
			createMessage repositories.CreateTaskMessage
			taskRecord    repositories.TaskRecord
			createErr     error
		)

		BeforeEach(func() {
			conditionAwaiter.AwaitConditionStub = func(ctx context.Context, _ client.WithWatch, object client.Object, _ string) (*korifiv1alpha1.CFTask, error) {
				cfTask, ok := object.(*korifiv1alpha1.CFTask)
				Expect(ok).To(BeTrue())

				Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
					cfTask.Status.SequenceID = 4
					cfTask.Status.MemoryMB = 256
					cfTask.Status.DiskQuotaMB = 128
					cfTask.Status.DropletRef = corev1.LocalObjectReference{
						Name: cfApp.Spec.CurrentDropletRef.Name,
					}
					meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.TaskInitializedConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "foo",
						Message: "bar",
					})
				})).To(Succeed())

				return cfTask, nil
			}

			createMessage = repositories.CreateTaskMessage{
				Command:   "echo 'hello world'",
				SpaceGUID: space.Name,
				AppGUID:   cfApp.Name,
				Metadata: repositories.Metadata{
					Labels:      map[string]string{"color": "blue"},
					Annotations: map[string]string{"extra-bugs": "true"},
				},
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

			It("awaits the initialized condition", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
				actualObject, actualCondition := conditionAwaiter.AwaitConditionArgsForCall(0)
				Expect(actualObject.GetNamespace()).To(Equal(space.Name))
				Expect(actualObject.GetName()).To(Equal(taskRecord.GUID))
				Expect(actualCondition).To(Equal(korifiv1alpha1.TaskInitializedConditionType))
			})

			It("creates the task", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(taskRecord.Name).NotTo(BeEmpty())
				Expect(taskRecord.GUID).NotTo(BeEmpty())
				Expect(taskRecord.Command).To(Equal("echo 'hello world'"))
				Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(taskRecord.SequenceID).To(Equal(int64(4)))

				Expect(taskRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(taskRecord.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

				Expect(taskRecord.MemoryMB).To(BeEquivalentTo(256))
				Expect(taskRecord.DiskMB).To(BeEquivalentTo(128))
				Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(taskRecord.State).To(Equal(repositories.TaskStatePending))
				Expect(taskRecord.Labels).To(Equal(map[string]string{"color": "blue"}))
				Expect(taskRecord.Annotations).To(Equal(map[string]string{"extra-bugs": "true"}))
			})

			When("the task never becomes initialized", func() {
				BeforeEach(func() {
					conditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFTask{}, errors.New("timed-out-error"))
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("timed-out-error")))
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
			taskGUID = uuid.NewString()
			cfTask = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: "echo hello",
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfTask)).To(Succeed())
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

			When("the task is initialized", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
						cfTask.Status.SequenceID = 6
						cfTask.Status.MemoryMB = 256
						cfTask.Status.DiskQuotaMB = 128
						cfTask.Status.DropletRef = corev1.LocalObjectReference{
							Name: cfApp.Spec.CurrentDropletRef.Name,
						}
						meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.TaskInitializedConditionType,
							Status:  metav1.ConditionTrue,
							Reason:  "foo",
							Message: "bar",
						})
					})).To(Succeed())
				})

				It("returns the task", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(taskRecord.Name).To(Equal(taskGUID))
					Expect(taskRecord.GUID).NotTo(BeEmpty())
					Expect(taskRecord.Command).To(Equal("echo hello"))
					Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
					Expect(taskRecord.SequenceID).To(BeEquivalentTo(6))

					Expect(taskRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					Expect(taskRecord.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

					Expect(taskRecord.MemoryMB).To(BeEquivalentTo(256))
					Expect(taskRecord.DiskMB).To(BeEquivalentTo(128))
					Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
					Expect(taskRecord.State).To(Equal(repositories.TaskStatePending))
				})

				When("the task is running", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
							meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.TaskStartedConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "foo",
								Message: "bar",
							})
						})).To(Succeed())
					})

					It("returns the running task", func() {
						Expect(getErr).NotTo(HaveOccurred())
						Expect(taskRecord.State).To(Equal(repositories.TaskStateRunning))
					})
				})

				When("the task has succeeded", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
							meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.TaskSucceededConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "foo",
								Message: "bar",
							})
						})).To(Succeed())
					})

					It("returns the succeeded task", func() {
						Expect(getErr).NotTo(HaveOccurred())
						Expect(taskRecord.State).To(Equal(repositories.TaskStateSucceeded))
					})
				})

				When("the task has failed", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
							meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.TaskStartedConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "foo",
								Message: "bar",
							})
							meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.TaskFailedConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "foo",
								Message: "bar",
							})
						})).To(Succeed())
					})

					It("returns the failed task", func() {
						Expect(getErr).NotTo(HaveOccurred())
						Expect(taskRecord.State).To(Equal(repositories.TaskStateFailed))
						Expect(taskRecord.FailureReason).To(Equal("bar"))
					})
				})

				When("the task was cancelled", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
							meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.TaskStartedConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "foo",
								Message: "bar",
							})
							meta.SetStatusCondition(&(cfTask.Status.Conditions), metav1.Condition{
								Type:   korifiv1alpha1.TaskFailedConditionType,
								Status: metav1.ConditionTrue,
								Reason: "TaskCanceled",
							})
						})).To(Succeed())
					})

					It("returns the failed task", func() {
						Expect(getErr).NotTo(HaveOccurred())
						Expect(taskRecord.State).To(Equal(repositories.TaskStateFailed))
						Expect(taskRecord.FailureReason).To(Equal("task was cancelled"))
					})
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
					Command: "echo hello",
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
					Command: "echo hello",
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

		It("returns an empty list due to no permissions", func() {
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
						Expect(k8s.Patch(ctx, k8sClient, task2, func() {
							task2.Status.SequenceID = 2
							task2.Status.DropletRef = corev1.LocalObjectReference{
								Name: cfApp2.Spec.CurrentDropletRef.Name,
							}
						})).To(Succeed())

						task21 := &korifiv1alpha1.CFTask{
							ObjectMeta: metav1.ObjectMeta{
								Name:      prefixedGUID("task21"),
								Namespace: space2.Name,
							},
							Spec: korifiv1alpha1.CFTaskSpec{
								Command: "echo hello",
								AppRef: corev1.LocalObjectReference{
									Name: cfApp2.Name,
								},
							},
						}
						Expect(k8sClient.Create(context.Background(), task21)).To(Succeed())
						Expect(k8s.Patch(ctx, k8sClient, task21, func() {
							task21.Status.SequenceID = 21
							task21.Status.DropletRef = corev1.LocalObjectReference{
								Name: cfApp2.Spec.CurrentDropletRef.Name,
							}
						})).To(Succeed())

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
			taskGUID = uuid.NewString()
			cfTask := &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: "echo hello",
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfTask)).To(Succeed())
			Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
				cfTask.Status.SequenceID = 6
				cfTask.Status.MemoryMB = 256
				cfTask.Status.DiskQuotaMB = 128
				cfTask.Status.DropletRef.Name = cfApp.Spec.CurrentDropletRef.Name
			})).To(Succeed())

			conditionAwaiter.AwaitConditionStub = func(ctx context.Context, _ client.WithWatch, object client.Object, _ string) (*korifiv1alpha1.CFTask, error) {
				cfTask, ok := object.(*korifiv1alpha1.CFTask)
				Expect(ok).To(BeTrue())

				Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
					meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.TaskCanceledConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "foo",
						Message: "bar",
					})
				})).To(Succeed())

				return cfTask, nil
			}
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

			It("awaits the cancelled condition", func() {
				Expect(cancelErr).NotTo(HaveOccurred())

				Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
				actualObject, actualCondition := conditionAwaiter.AwaitConditionArgsForCall(0)
				Expect(actualObject.GetNamespace()).To(Equal(space.Name))
				Expect(actualObject.GetName()).To(Equal(taskRecord.GUID))
				Expect(actualCondition).To(Equal(korifiv1alpha1.TaskCanceledConditionType))
			})

			It("returns a cancelled task record", func() {
				Expect(cancelErr).NotTo(HaveOccurred())
				Expect(taskRecord.Name).To(Equal(taskGUID))
				Expect(taskRecord.GUID).NotTo(BeEmpty())
				Expect(taskRecord.Command).To(Equal("echo hello"))
				Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(taskRecord.SequenceID).To(BeEquivalentTo(6))

				Expect(taskRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(taskRecord.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

				Expect(taskRecord.MemoryMB).To(BeEquivalentTo(256))
				Expect(taskRecord.DiskMB).To(BeEquivalentTo(128))
				Expect(taskRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(taskRecord.State).To(Equal(repositories.TaskStateCanceling))
			})

			When("the status is not updated within the timeout", func() {
				BeforeEach(func() {
					conditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFTask{}, errors.New("timed-out"))
				})

				It("returns a timeout error", func() {
					Expect(cancelErr).To(MatchError(ContainSubstring("timed-out")))
				})
			})
		})
	})

	Describe("PatchTaskMetadata", func() {
		var (
			cfTask                        *korifiv1alpha1.CFTask
			taskGUID                      string
			patchErr                      error
			taskRecord                    repositories.TaskRecord
			labelsPatch, annotationsPatch map[string]*string
		)

		BeforeEach(func() {
			taskGUID = uuid.NewString()
			cfTask = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: "echo hello",
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfTask)).To(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, cfTask, func() {
				cfTask.Status.SequenceID = 6
				cfTask.Status.MemoryMB = 256
				cfTask.Status.DiskQuotaMB = 128
				cfTask.Status.DropletRef = corev1.LocalObjectReference{
					Name: cfApp.Spec.CurrentDropletRef.Name,
				}
			})).To(Succeed())

			labelsPatch = nil
			annotationsPatch = nil
		})

		JustBeforeEach(func() {
			patchMsg := repositories.PatchTaskMetadataMessage{
				TaskGUID:  taskGUID,
				SpaceGUID: space.Name,
				MetadataPatch: repositories.MetadataPatch{
					Annotations: annotationsPatch,
					Labels:      labelsPatch,
				},
			}

			taskRecord, patchErr = taskRepo.PatchTaskMetadata(ctx, authInfo, patchMsg)
		})

		When("the user is authorized and the task exists", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("the task doesn't have labels or annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					annotationsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					Expect(k8s.PatchResource(ctx, k8sClient, cfTask, func() {
						cfTask.Labels = nil
						cfTask.Annotations = nil
					})).To(Succeed())
				})

				It("returns the updated org record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(taskRecord.GUID).To(Equal(taskGUID))
					Expect(taskRecord.SpaceGUID).To(Equal(space.Name))
					Expect(taskRecord.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(taskRecord.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})

				It("sets the k8s CFSpace resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFTask := new(korifiv1alpha1.CFTask)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfTask), updatedCFTask)).To(Succeed())
					Expect(updatedCFTask.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(updatedCFTask.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})
			})

			When("the task already has labels and annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one":        tools.PtrTo("value-one-updated"),
						"key-two":        tools.PtrTo("value-two"),
						"before-key-two": nil,
					}
					annotationsPatch = map[string]*string{
						"key-one":        tools.PtrTo("value-one-updated"),
						"key-two":        tools.PtrTo("value-two"),
						"before-key-two": nil,
					}
					Expect(k8s.PatchResource(ctx, k8sClient, cfTask, func() {
						cfTask.Labels = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
						cfTask.Annotations = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
					})).To(Succeed())
				})

				It("returns the updated task record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(taskRecord.GUID).To(Equal(cfTask.Name))
					Expect(taskRecord.SpaceGUID).To(Equal(cfTask.Namespace))
					Expect(taskRecord.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(taskRecord.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})

				It("sets the k8s cftask resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFTask := new(korifiv1alpha1.CFTask)
					Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfTask), updatedCFTask)).To(Succeed())
					Expect(updatedCFTask.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(updatedCFTask.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})
			})

			When("an annotation is invalid", func() {
				BeforeEach(func() {
					annotationsPatch = map[string]*string{
						"-bad-annotation": tools.PtrTo("stuff"),
					}
				})

				It("returns an UnprocessableEntityError", func() {
					var unprocessableEntityError apierrors.UnprocessableEntityError
					Expect(errors.As(patchErr, &unprocessableEntityError)).To(BeTrue())
					Expect(unprocessableEntityError.Detail()).To(SatisfyAll(
						ContainSubstring("metadata.annotations is invalid"),
						ContainSubstring(`"-bad-annotation"`),
						ContainSubstring("alphanumeric"),
					))
				})
			})

			When("a label is invalid", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"-bad-label": tools.PtrTo("stuff"),
					}
				})

				It("returns an UnprocessableEntityError", func() {
					var unprocessableEntityError apierrors.UnprocessableEntityError
					Expect(errors.As(patchErr, &unprocessableEntityError)).To(BeTrue())
					Expect(unprocessableEntityError.Detail()).To(SatisfyAll(
						ContainSubstring("metadata.labels is invalid"),
						ContainSubstring(`"-bad-label"`),
						ContainSubstring("alphanumeric"),
					))
				})
			})
		})

		When("the user is authorized but the task does not exist", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				taskGUID = "invalidTaskName"
			})

			It("fails to get the task", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the user is not authorized", func() {
			It("return a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})
})
