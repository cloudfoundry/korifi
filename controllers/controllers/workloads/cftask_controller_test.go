package workloads_test

import (
	"context"
	"errors"
	"time"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	workloadsfake "code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	"code.cloudfoundry.org/korifi/controllers/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFTask Controller", func() {
	var (
		taskReconciler     workloads.CFTaskReconciler
		k8sClient          *fake.Client
		statusWriter       *fake.StatusWriter
		eventRecorder      *fake.EventRecorder
		seqIdGenerator     *workloadsfake.SeqIdGenerator
		envBuilder         *workloadsfake.EnvBuilder
		result             controllerruntime.Result
		err                error
		req                controllerruntime.Request
		dropletRef         string
		cftaskGetError     error
		cfappGetError      error
		cfdropletGetError  error
		droplet            *korifiv1alpha1.BuildDropletStatus
		cfTask             korifiv1alpha1.CFTask
		eiriniTask         eiriniv1.Task
		eiriniTaskGetError error
		taskTTL            time.Duration
		processList        []korifiv1alpha1.CFProcess
	)

	BeforeEach(func() {
		k8sClient = new(fake.Client)
		statusWriter = new(fake.StatusWriter)
		k8sClient.StatusReturns(statusWriter)

		eventRecorder = new(fake.EventRecorder)
		seqIdGenerator = new(workloadsfake.SeqIdGenerator)
		seqIdGenerator.GenerateReturns(314, nil)
		envBuilder = new(workloadsfake.EnvBuilder)
		envBuilder.BuildEnvReturns([]corev1.EnvVar{{Name: "FOO", Value: "bar"}}, nil)
		logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		taskTTL = 5 * time.Minute
		taskReconciler = *workloads.NewCFTaskReconciler(
			k8sClient,
			scheme.Scheme,
			eventRecorder,
			logger,
			seqIdGenerator,
			envBuilder,
			config.CFProcessDefaults{
				MemoryMB:    256,
				DiskQuotaMB: 128,
			},
			taskTTL,
		)

		cftaskGetError = nil
		cfappGetError = nil
		cfdropletGetError = nil
		eiriniTaskGetError = k8serrors.NewNotFound(schema.GroupResource{}, "not-found")
		dropletRef = "the-droplet-guid"
		droplet = &korifiv1alpha1.BuildDropletStatus{
			Registry: korifiv1alpha1.Registry{
				Image:            "the-image",
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-secret"}},
			},
		}
		cfTask = korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name: "the-task-guid",
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				Command: "echo hello",
				AppRef:  corev1.LocalObjectReference{Name: "the-app-guid"},
			},
		}
		eiriniTask = eiriniv1.Task{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "the-task-namespace",
				Name:      cfTask.Name,
			},
			Spec: eiriniv1.TaskSpec{
				Name: cfTask.Name,
			},
		}
		req = controllerruntime.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "the-task-namespace",
				Name:      "the-task-name",
			},
		}
		k8sClient.GetStub = func(_ context.Context, namespacedName types.NamespacedName, obj client.Object) error {
			switch t := obj.(type) {
			case *korifiv1alpha1.CFTask:
				*t = *cfTask.DeepCopy()
				t.Namespace = namespacedName.Namespace

				return cftaskGetError
			case *korifiv1alpha1.CFApp:
				Expect(namespacedName.Name).To(Equal("the-app-guid"))
				*t = korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      namespacedName.Name,
						Namespace: namespacedName.Namespace,
					},
					Spec: korifiv1alpha1.CFAppSpec{
						CurrentDropletRef: corev1.LocalObjectReference{Name: dropletRef},
					},
				}
				return cfappGetError
			case *korifiv1alpha1.CFBuild:
				Expect(namespacedName.Name).To(Equal(dropletRef))
				*t = korifiv1alpha1.CFBuild{
					ObjectMeta: metav1.ObjectMeta{
						Name:      namespacedName.Name,
						Namespace: namespacedName.Namespace,
					},
					Status: korifiv1alpha1.CFBuildStatus{
						Droplet: droplet,
					},
				}
				return cfdropletGetError
			case *eiriniv1.Task:
				Expect(namespacedName.Name).To(Equal("the-task-guid"))
				*t = eiriniTask

				return eiriniTaskGetError
			}

			return nil
		}

		processList = []korifiv1alpha1.CFProcess{{
			Spec: korifiv1alpha1.CFProcessSpec{MemoryMB: 512},
		}}

		k8sClient.ListStub = func(_ context.Context, l client.ObjectList, options ...client.ListOption) error {
			switch t := l.(type) {
			case *korifiv1alpha1.CFProcessList:
				Expect(options).To(ConsistOf(
					client.InNamespace("the-task-namespace"),
					client.MatchingLabels{
						korifiv1alpha1.CFProcessTypeLabelKey: "web",
						korifiv1alpha1.CFAppGUIDLabelKey:     "the-app-guid",
					},
				))

				*t = korifiv1alpha1.CFProcessList{
					Items: processList,
				}
			}

			return nil
		}
	})

	JustBeforeEach(func() {
		result, err = taskReconciler.Reconcile(context.Background(), req)
	})

	taskWithPatchedStatus := func() *korifiv1alpha1.CFTask {
		ExpectWithOffset(1, statusWriter.PatchCallCount()).To(Equal(1))
		_, object, _, _ := statusWriter.PatchArgsForCall(0)
		task, ok := object.(*korifiv1alpha1.CFTask)
		ExpectWithOffset(1, ok).To(BeTrue())

		return task
	}

	Describe("task creation", func() {
		It("creates an eirini.Task correctly", func() {
			Expect(envBuilder.BuildEnvCallCount()).To(Equal(1))
			_, envApp := envBuilder.BuildEnvArgsForCall(0)
			Expect(envApp.Name).To(Equal("the-app-guid"))

			Expect(k8sClient.CreateCallCount()).To(Equal(1))
			_, obj, _ := k8sClient.CreateArgsForCall(0)
			eiriniTask, ok := obj.(*eiriniv1.Task)
			Expect(ok).To(BeTrue())

			Expect(eiriniTask.Name).To(Equal("the-task-guid"))
			Expect(eiriniTask.Namespace).To(Equal("the-task-namespace"))
			Expect(eiriniTask.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFTaskGUIDLabelKey, "the-task-guid"))
			Expect(eiriniTask.Spec.Image).To(Equal("the-image"))
			Expect(eiriniTask.Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{{Name: "registry-secret"}}))
			Expect(eiriniTask.Spec.Command).To(Equal([]string{"/cnb/lifecycle/launcher", "echo hello"}))
			Expect(eiriniTask.Spec.MemoryMB).To(BeEquivalentTo(256))
			Expect(eiriniTask.Spec.DiskMB).To(BeEquivalentTo(128))
			Expect(eiriniTask.Spec.CPUMillis).To(BeEquivalentTo(50))
			Expect(eiriniTask.Spec.Environment).To(ConsistOf(corev1.EnvVar{Name: "FOO", Value: "bar"}))

			eiriniTaskOwner := metav1.GetControllerOf(eiriniTask)
			Expect(eiriniTaskOwner).NotTo(BeNil())
			Expect(eiriniTaskOwner.UID).To(Equal(cfTask.UID))
		})

		When("setting the owner reference fails", func() {
			BeforeEach(func() {
				// setting controller reference on object that is already owned by a controller yields an error
				eiriniTask.OwnerReferences = []metav1.OwnerReference{{
					Controller: pointer.Bool(true),
				}}
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("the eirini task exists", func() {
			BeforeEach(func() {
				eiriniTaskGetError = nil
			})

			It("patches the task", func() {
				Expect(k8sClient.PatchCallCount()).To(Equal(1))
				_, actualObject, _, _ := k8sClient.PatchArgsForCall(0)
				actualEiriniTask, ok := actualObject.(*eiriniv1.Task)
				Expect(ok).To(BeTrue())

				Expect(actualEiriniTask.Name).To(Equal("the-task-guid"))
				Expect(actualEiriniTask.Namespace).To(Equal("the-task-namespace"))
				Expect(actualEiriniTask.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFTaskGUIDLabelKey, "the-task-guid"))
				Expect(actualEiriniTask.Spec.Image).To(Equal("the-image"))
				Expect(actualEiriniTask.Spec.Command).To(Equal([]string{"/cnb/lifecycle/launcher", "echo hello"}))
				Expect(actualEiriniTask.Spec.MemoryMB).To(BeNumerically("==", 256))
				Expect(actualEiriniTask.Spec.DiskMB).To(BeNumerically("==", 128))
			})

			It("does not record task created event", func() {
				Expect(eventRecorder.EventfCallCount()).To(BeZero())
			})

			When("a label exists", func() {
				BeforeEach(func() {
					eiriniTask.Labels = map[string]string{"foo": "bar"}
				})

				It("preserves existing labels", func() {
					Expect(k8sClient.PatchCallCount()).To(Equal(1))
					_, actualObject, _, _ := k8sClient.PatchArgsForCall(0)
					actualEiriniTask, ok := actualObject.(*eiriniv1.Task)
					Expect(ok).To(BeTrue())

					Expect(actualEiriniTask.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFTaskGUIDLabelKey, "the-task-guid"))
					Expect(actualEiriniTask.Labels).To(HaveKeyWithValue("foo", "bar"))
				})
			})

			When("the eirini task has started", func() {
				var now metav1.Time

				BeforeEach(func() {
					now = metav1.Now()

					meta.SetStatusCondition(&eiriniTask.Status.Conditions, metav1.Condition{
						Type:               eiriniv1.TaskStartedConditionType,
						Status:             metav1.ConditionTrue,
						Reason:             "eirini-task-started",
						Message:            "eirini task started",
						LastTransitionTime: now,
					})
				})

				It("sets the started condition on the korifi task", func() {
					startedCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskStartedConditionType)
					Expect(startedCondition).NotTo(BeNil())
					Expect(startedCondition.Status).To(Equal(metav1.ConditionTrue))
					Expect(startedCondition.Reason).To(Equal("eirini-task-started"))
					Expect(startedCondition.Message).To(Equal("eirini task started"))
					Expect(startedCondition.LastTransitionTime).To(Equal(now))
				})

				It("does not requeue the task", func() {
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			When("the eirini task has succeeded", func() {
				var now metav1.Time

				BeforeEach(func() {
					now = metav1.NewTime(time.Now().Add(-2 * time.Second))

					meta.SetStatusCondition(&eiriniTask.Status.Conditions, metav1.Condition{
						Type:               eiriniv1.TaskSucceededConditionType,
						Status:             metav1.ConditionTrue,
						Reason:             "eirini-task-succeeded",
						Message:            "eirini task succeeded",
						LastTransitionTime: now,
					})
				})

				It("sets the succeeded condition on the korifi task", func() {
					succeededCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskSucceededConditionType)
					Expect(succeededCondition).NotTo(BeNil())
					Expect(succeededCondition.Status).To(Equal(metav1.ConditionTrue))
					Expect(succeededCondition.Reason).To(Equal("eirini-task-succeeded"))
					Expect(succeededCondition.Message).To(Equal("eirini task succeeded"))
					Expect(succeededCondition.LastTransitionTime).To(Equal(now))
				})

				It("requeues the task adding TTL", func() {
					nowPlusTTL := now.Add(taskTTL)
					Expect(result.RequeueAfter).ToNot(BeZero())
					currentPlusRequeueAfter := time.Now().Add(result.RequeueAfter)

					Expect(currentPlusRequeueAfter).To(BeTemporally("~", nowPlusTTL, time.Second))
				})
			})

			When("the eirini task has failed", func() {
				var now metav1.Time

				BeforeEach(func() {
					now = metav1.NewTime(time.Now().Add(-2 * time.Second))

					meta.SetStatusCondition(&eiriniTask.Status.Conditions, metav1.Condition{
						Type:               eiriniv1.TaskFailedConditionType,
						Status:             metav1.ConditionTrue,
						Reason:             "eirini-task-failed",
						Message:            "eirini task failed",
						LastTransitionTime: now,
					})
				})

				It("sets the failed condition on the korifi task", func() {
					failedCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskFailedConditionType)
					Expect(failedCondition).NotTo(BeNil())
					Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
					Expect(failedCondition.Reason).To(Equal("eirini-task-failed"))
					Expect(failedCondition.Message).To(Equal("eirini task failed"))
					Expect(failedCondition.LastTransitionTime).To(Equal(now))
				})

				It("requeues the task adding TTL", func() {
					nowPlusTTL := now.Add(taskTTL)
					Expect(result.RequeueAfter).ToNot(BeZero())
					currentPlusRequeueAfter := time.Now().Add(result.RequeueAfter)

					Expect(currentPlusRequeueAfter).To(BeTemporally("~", nowPlusTTL, time.Second))
				})
			})
		})

		It("emits a normal event for successful reconciliation", func() {
			Expect(eventRecorder.EventfCallCount()).To(Equal(1))
			obj, eventType, reason, message, _ := eventRecorder.EventfArgsForCall(0)
			task, ok := obj.(*korifiv1alpha1.CFTask)
			Expect(ok).To(BeTrue())
			Expect(task.Name).To(Equal("the-task-guid"))
			Expect(eventType).To(Equal("Normal"))
			Expect(reason).To(Equal("taskCreated"))
			Expect(message).To(ContainSubstring("Created eirini task %s"))
		})

		It("populates the CFTask Status", func() {
			task := taskWithPatchedStatus()

			Expect(task.Status.DiskQuotaMB).To(BeNumerically("==", 128))
			Expect(task.Status.MemoryMB).To(BeNumerically("==", 256))
			Expect(task.Status.SequenceID).To(BeNumerically("==", 314))
			Expect(task.Status.DropletRef.Name).To(Equal("the-droplet-guid"))
			Expect(meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue())
		})

		When("creating the eirini task fails", func() {
			BeforeEach(func() {
				k8sClient.CreateReturns(errors.New("create-eirini-task-failure"))
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})

			It("still sets the initialized task status condition", func() {
				Expect(meta.IsStatusConditionTrue(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue())
			})
		})

		When("Status.SequenceID has been already set", func() {
			BeforeEach(func() {
				cfTask.Status.SequenceID = 5
			})

			It("does not update the sequence id", func() {
				task := taskWithPatchedStatus()
				Expect(task.Status.SequenceID).To(BeEquivalentTo(5))
			})
		})

		When("getting the cftask returns an error", func() {
			BeforeEach(func() {
				cftaskGetError = errors.New("boom")
			})

			It("requeues the reconciliation", func() {
				Expect(err).To(HaveOccurred())
			})

			When("the errors is a NotFound error", func() {
				BeforeEach(func() {
					cftaskGetError = k8serrors.NewNotFound(schema.GroupResource{}, "")
				})

				It("does not retry the reconciliation", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})
		})

		When("generating the sequence ID fails", func() {
			BeforeEach(func() {
				seqIdGenerator.GenerateReturns(0, errors.New("seq-id"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("seq-id"))
			})
		})

		When("patching the CFTask status fails", func() {
			BeforeEach(func() {
				statusWriter.PatchReturns(errors.New("status-patch"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("status-patch"))
			})
		})

		When("getting the app returns an error", func() {
			BeforeEach(func() {
				cfappGetError = errors.New("boom")
			})

			It("retries the reconciliation", func() {
				Expect(err).To(HaveOccurred())
			})

			When("the app is not found", func() {
				BeforeEach(func() {
					cfappGetError = k8serrors.NewNotFound(schema.GroupResource{}, "")
				})

				It("writes a warning event", func() {
					Expect(eventRecorder.EventfCallCount()).To(Equal(1))
					obj, eventType, reason, message, _ := eventRecorder.EventfArgsForCall(0)
					task, ok := obj.(*korifiv1alpha1.CFTask)
					Expect(ok).To(BeTrue())
					Expect(task.Name).To(Equal("the-task-guid"))
					Expect(eventType).To(Equal("Warning"))
					Expect(reason).To(Equal("appNotFound"))
					Expect(message).To(ContainSubstring("Did not find app with name"))
				})
			})
		})

		When("the app does not have a current droplet ref set", func() {
			BeforeEach(func() {
				dropletRef = ""
			})

			It("requeues and writes a warning event", func() {
				Expect(err).To(HaveOccurred())
				Expect(eventRecorder.EventfCallCount()).To(Equal(1))
				obj, eventType, reason, message, _ := eventRecorder.EventfArgsForCall(0)
				task, ok := obj.(*korifiv1alpha1.CFTask)
				Expect(ok).To(BeTrue())
				Expect(task.Name).To(Equal("the-task-guid"))
				Expect(eventType).To(Equal("Warning"))
				Expect(reason).To(Equal("appCurrentDropletRefNotSet"))
				Expect(message).To(ContainSubstring("does not have a current droplet"))
			})
		})

		When("getting the droplet returns an error", func() {
			BeforeEach(func() {
				cfdropletGetError = errors.New("boom")
			})

			It("retries without an event", func() {
				Expect(err).To(HaveOccurred())
				Expect(eventRecorder.EventfCallCount()).To(Equal(0))
			})

			When("the droplet does not exist", func() {
				BeforeEach(func() {
					cfdropletGetError = k8serrors.NewNotFound(schema.GroupResource{}, "")
				})

				It("requeues and writes a warning event", func() {
					Expect(err).To(HaveOccurred())
					Expect(eventRecorder.EventfCallCount()).To(Equal(1))
					obj, eventType, reason, message, _ := eventRecorder.EventfArgsForCall(0)
					task, ok := obj.(*korifiv1alpha1.CFTask)
					Expect(ok).To(BeTrue())
					Expect(task.Name).To(Equal("the-task-guid"))
					Expect(eventType).To(Equal("Warning"))
					Expect(reason).To(Equal("appCurrentDropletNotFound"))
					Expect(message).To(ContainSubstring("Current droplet %s for app %s does not exist"))
				})
			})
		})

		When("the droplet does not have droplet set in its status", func() {
			BeforeEach(func() {
				droplet = nil
			})

			It("requeues and writes a warning event", func() {
				Expect(err).To(HaveOccurred())
				Expect(eventRecorder.EventfCallCount()).To(Equal(1))
				obj, eventType, reason, message, _ := eventRecorder.EventfArgsForCall(0)
				task, ok := obj.(*korifiv1alpha1.CFTask)
				Expect(ok).To(BeTrue())
				Expect(task.Name).To(Equal("the-task-guid"))
				Expect(eventType).To(Equal("Warning"))
				Expect(reason).To(Equal("dropletBuildStatusNotSet"))
				Expect(message).To(ContainSubstring("Current droplet %s from app %s does not have a droplet image"))
			})
		})

		When("it fails to retrieve the app's web process", func() {
			BeforeEach(func() {
				k8sClient.ListReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("when the app doesn't have exactly one web process", func() {
			BeforeEach(func() {
				processList = nil
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("building the environment fails", func() {
			BeforeEach(func() {
				envBuilder.BuildEnvReturns(nil, errors.New("oops"))
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("task cancellation", func() {
		BeforeEach(func() {
			cfTask.Spec.Canceled = true
		})

		It("deletes the underlying eirini task", func() {
			Expect(k8sClient.DeleteCallCount()).To(Equal(1))
			_, actualObject, _ := k8sClient.DeleteArgsForCall(0)
			actualEiriniTask, ok := actualObject.(*eiriniv1.Task)
			Expect(ok).To(BeTrue())

			Expect(actualEiriniTask.Namespace).To(Equal("the-task-namespace"))
			Expect(actualEiriniTask.Name).To(Equal("the-task-guid"))
		})

		When("processing cancellation and the eirini task does not exist", func() {
			BeforeEach(func() {
				eiriniTaskGetError = k8serrors.NewNotFound(schema.GroupResource{}, "eirini-task")
			})

			It("does not attempt to create the eirini task", func() {
				Expect(k8sClient.CreateCallCount()).To(BeZero())
			})
		})

		It("sets the canceled condition on the korifi task", func() {
			canceledCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskCanceledConditionType)
			Expect(canceledCondition).NotTo(BeNil())
			Expect(canceledCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(canceledCondition.Reason).To(Equal("taskCanceled"))
		})

		When("the task is not completed", func() {
			BeforeEach(func() {
				meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
					Type:    korifiv1alpha1.TaskStartedConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  "foo",
					Message: "bar",
				})
			})

			It("sets the failed condition", func() {
				failedCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskFailedConditionType)
				Expect(failedCondition).NotTo(BeNil())
				Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(failedCondition.Reason).To(Equal("taskCanceled"))
			})
		})

		When("the task has succeeded", func() {
			BeforeEach(func() {
				meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
					Type:    korifiv1alpha1.TaskSucceededConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  "foo",
					Message: "bar",
				})
			})

			It("does not set the failed condition", func() {
				failedCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskFailedConditionType)
				Expect(failedCondition).To(BeNil())
			})
		})

		When("deleting of the underlying eirini task fails", func() {
			BeforeEach(func() {
				k8sClient.DeleteReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("boom"))
			})

			It("does not set the canceled condition on the korifi task", func() {
				canceledCondition := meta.FindStatusCondition(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskCanceledConditionType)
				Expect(canceledCondition).To(BeNil())
			})
		})

		When("underlying eirini task does not exist", func() {
			BeforeEach(func() {
				k8sClient.DeleteReturns(k8serrors.NewNotFound(schema.GroupResource{}, "eirini-task"))
			})

			It("sets the canceled condition on the korifi task", func() {
				Expect(meta.IsStatusConditionTrue(taskWithPatchedStatus().Status.Conditions, korifiv1alpha1.TaskCanceledConditionType)).To(BeTrue())
			})
		})
	})

	Describe("task expiration", func() {
		BeforeEach(func() {
			meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
				Type:               korifiv1alpha1.TaskSucceededConditionType,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * taskTTL)),
				Reason:             "succeeded",
				Message:            "succeeded",
			})
		})

		It("deletes the task", func() {
			Expect(k8sClient.DeleteCallCount()).To(Equal(1))
			_, deletedObject, _ := k8sClient.DeleteArgsForCall(0)
			Expect(deletedObject.GetNamespace()).To(Equal("the-task-namespace"))
			Expect(deletedObject.GetName()).To(Equal(cfTask.Name))
		})
	})
})
