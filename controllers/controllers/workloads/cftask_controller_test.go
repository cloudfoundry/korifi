package workloads_test

import (
	"context"
	"errors"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	workloadsfake "code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	"code.cloudfoundry.org/korifi/controllers/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFTask Controller", func() {
	var (
		taskReconciler workloads.CFTaskReconciler
		k8sClient      *fake.Client
		statusWriter   *fake.StatusWriter
		eventRecorder  *fake.EventRecorder
		seqIdGenerator *workloadsfake.SeqIdGenerator
	)

	BeforeEach(func() {
		k8sClient = new(fake.Client)
		statusWriter = new(fake.StatusWriter)
		k8sClient.StatusReturns(statusWriter)

		eventRecorder = new(fake.EventRecorder)
		seqIdGenerator = new(workloadsfake.SeqIdGenerator)
		seqIdGenerator.GenerateReturns(314, nil)
		logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
		taskReconciler = *workloads.NewCFTaskReconciler(k8sClient, scheme.Scheme, eventRecorder, logger, seqIdGenerator)
	})

	Describe("task creation", func() {
		var (
			result            controllerruntime.Result
			err               error
			req               controllerruntime.Request
			dropletRef        string
			cftaskGetError    error
			cfappGetError     error
			cfdropletGetError error
			droplet           *korifiv1alpha1.BuildDropletStatus
			cfTask            korifiv1alpha1.CFTask
		)

		BeforeEach(func() {
			cftaskGetError = nil
			cfappGetError = nil
			cfdropletGetError = nil
			dropletRef = "the-droplet-guid"
			droplet = &korifiv1alpha1.BuildDropletStatus{
				Registry: korifiv1alpha1.Registry{
					Image: "the-image",
				},
			}
			cfTask = korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name: "the-task-guid",
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: []string{"echo", "hello"},
					AppRef:  corev1.LocalObjectReference{Name: "the-app-guid"},
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
						Spec: korifiv1alpha1.CFAppSpec{
							CurrentDropletRef: corev1.LocalObjectReference{Name: dropletRef},
						},
					}
					return cfappGetError
				case *korifiv1alpha1.CFBuild:
					Expect(namespacedName.Name).To(Equal(dropletRef))
					*t = korifiv1alpha1.CFBuild{
						Status: korifiv1alpha1.CFBuildStatus{
							Droplet: droplet,
						},
					}
					return cfdropletGetError
				}

				return nil
			}
		})

		JustBeforeEach(func() {
			result, err = taskReconciler.Reconcile(context.Background(), req)
		})

		It("creates an eirini.Task correctly", func() {
			Expect(k8sClient.CreateCallCount()).To(Equal(1))

			_, obj, _ := k8sClient.CreateArgsForCall(0)
			eiriniTask, ok := obj.(*eiriniv1.Task)
			Expect(ok).To(BeTrue())

			Expect(eiriniTask.Name).To(Equal("the-task-guid"))
			Expect(eiriniTask.Namespace).To(Equal("the-task-namespace"))
			Expect(eiriniTask.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFTaskGUIDLabelKey, "the-task-guid"))
			Expect(eiriniTask.Spec.Command).To(ConsistOf("echo", "hello"))
			Expect(eiriniTask.Spec.Image).To(Equal("the-image"))
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

		It("initialises Status.SequenceID", func() {
			Expect(statusWriter.PatchCallCount()).To(Equal(1))
			_, object, patch, _ := statusWriter.PatchArgsForCall(0)
			patchBytes, patchErr := patch.Data(object)
			Expect(patchErr).NotTo(HaveOccurred())
			Expect(string(patchBytes)).To(MatchJSON(`{"status":{"sequenceId":314}}`))
		})

		When("Status.SequenceID has been already set", func() {
			BeforeEach(func() {
				cfTask.Status.SequenceID = 5
			})

			It("does not update the sequence id", func() {
				Expect(statusWriter.PatchCallCount()).To(BeZero())
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

		When("eirini task already exists with expected metadata.name", func() {
			BeforeEach(func() {
				k8sClient.CreateReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, ""))
			})

			It("does not requeue", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})
	})
})
