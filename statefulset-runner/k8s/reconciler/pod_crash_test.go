package reconciler_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/k8sfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler/reconcilerfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("K8s/Reconciler/AppCrash", func() {
	var (
		podCrashReconciler  *reconciler.PodCrash
		crashEventGenerator *reconcilerfakes.FakeCrashEventGenerator
		client              *k8sfakes.FakeClient
		resultErr           error
		podOwners           []metav1.OwnerReference
		podGetError         error
		podAnnotations      map[string]string
		statefulSet         appsv1.StatefulSet
		getStSetError       error
	)

	BeforeEach(func() {
		crashEventGenerator = new(reconcilerfakes.FakeCrashEventGenerator)
		client = new(k8sfakes.FakeClient)
		podCrashReconciler = reconciler.NewPodCrash(
			tests.NewTestLogger("pod-crash-test"),
			client,
			crashEventGenerator,
		)
		podAnnotations = nil

		statefulSet = appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "parent-statefulset",
				Namespace: "some-ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "eirini/v1",
						Kind:       "LRP",
						Name:       "parent-lrp",
						UID:        "asdf",
					},
				},
			},
		}
		getStSetError = nil
		podGetError = nil

		client.GetStub = func(_ context.Context, _ types.NamespacedName, o k8sclient.Object) error {
			stSetPtr, ok := o.(*appsv1.StatefulSet)
			if ok {
				if getStSetError != nil {
					return getStSetError
				}
				statefulSet.DeepCopyInto(stSetPtr)

				return nil
			}

			podPtr, ok := o.(*corev1.Pod)
			if ok {
				if podGetError != nil {
					return podGetError
				}
				podPtr.Namespace = "some-ns"
				podPtr.Name = "app-instance"
				podPtr.OwnerReferences = podOwners
				podPtr.Annotations = podAnnotations

				return nil
			}

			Fail(fmt.Sprintf("Unsupported object: %v", o))

			return nil
		}

		podGetError = nil
		t := true
		podOwners = []metav1.OwnerReference{
			{
				Name:       "stateful-set-controller",
				UID:        "sdfp",
				Controller: &t,
			},
			{
				APIVersion: "v1",
				Kind:       "StatefulSet",
				Name:       "parent-statefulset",
				UID:        "sdfp",
			},
		}
	})

	JustBeforeEach(func() {
		_, resultErr = podCrashReconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "some-ns",
				Name:      "app-instance",
			},
		})
	})

	It("sends the correct pod info to the crash event generator", func() {
		Expect(resultErr).NotTo(HaveOccurred())
		Expect(client.GetCallCount()).To(Equal(1))
		_, nsName, _ := client.GetArgsForCall(0)
		Expect(nsName).To(Equal(types.NamespacedName{Namespace: "some-ns", Name: "app-instance"}))

		Expect(crashEventGenerator.GenerateCallCount()).To(Equal(1))
		_, pod, _ := crashEventGenerator.GenerateArgsForCall(0)
		Expect(pod.Namespace).To(Equal("some-ns"))
		Expect(pod.Name).To(Equal("app-instance"))
	})

	When("no crash is generated", func() {
		BeforeEach(func() {
			crashEventGenerator.GenerateReturns(nil)
		})

		It("does not create a k8s event", func() {
			Expect(client.CreateCallCount()).To(Equal(0))
		})
	})

	When("a crash has occurred", func() {
		var timestamp time.Time

		BeforeEach(func() {
			timestamp = time.Unix(time.Now().Unix(), 0)
			crashEventGenerator.GenerateReturns(&reconciler.CrashEvent{
				ProcessGUID:    "process-guid",
				Instance:       "instance-name",
				Index:          3,
				Reason:         "Error",
				ExitCode:       6,
				CrashTimestamp: timestamp.Unix(),
			})
		})

		It("creates a k8s event", func() {
			Expect(client.CreateCallCount()).To(Equal(1))
			_, obj, _ := client.CreateArgsForCall(0)
			event, ok := obj.(*corev1.Event)
			Expect(ok).To(BeTrue())

			Expect(event.Namespace).To(Equal("some-ns"))
			Expect(event.GenerateName).To(Equal("instance-name-"))
			Expect(event.Labels).To(HaveKeyWithValue("korifi.cloudfoundry.org/instance-index", "3"))
			Expect(event.Annotations).To(HaveKeyWithValue("korifi.cloudfoundry.org/process-guid", "process-guid"))

			Expect(event.LastTimestamp).To(Equal(metav1.NewTime(timestamp)))
			Expect(event.FirstTimestamp).To(Equal(metav1.NewTime(timestamp)))
			Expect(event.EventTime.Time).To(SatisfyAll(
				BeTemporally(">", timestamp),
				BeTemporally("<", time.Now()),
			))
			Expect(event.InvolvedObject).To(Equal(corev1.ObjectReference{
				Kind:            "LRP",
				Namespace:       "some-ns",
				Name:            "parent-lrp",
				UID:             "asdf",
				APIVersion:      "eirini/v1",
				ResourceVersion: "",
				FieldPath:       "spec.containers{opi}",
			}))
			Expect(event.Reason).To(Equal("Container: Error"))
			Expect(event.Message).To(Equal("Container terminated with exit code: 6"))
			Expect(event.Count).To(Equal(int32(1)))
			Expect(event.Source).To(Equal(corev1.EventSource{Component: "eirini-controller"}))
			Expect(event.Type).To(Equal("Warning"))
		})

		It("records the crash timestamp as an annotation on the pod", func() {
			Expect(client.PatchCallCount()).To(Equal(1))

			_, p, patch, _ := client.PatchArgsForCall(0)

			pd, ok := p.(*corev1.Pod)
			Expect(ok).To(BeTrue(), "didn't pass a *Pod to patch")

			Expect(pd.Name).To(Equal("app-instance"))
			Expect(pd.Namespace).To(Equal("some-ns"))

			patchBytes, err := patch.Data(p)
			Expect(err).NotTo(HaveOccurred())
			patchTimestamp := strconv.FormatInt(timestamp.Unix(), 10)
			Expect(string(patchBytes)).To(SatisfyAll(
				ContainSubstring(stset.AnnotationLastReportedLRPCrash),
				ContainSubstring(patchTimestamp),
			))
		})

		When("patching the pod errors", func() {
			BeforeEach(func() {
				client.PatchReturns(errors.New("boom"))
			})

			It("ignores the error", func() {
				Expect(resultErr).NotTo(HaveOccurred())
			})
		})

		When("the app crash has already been reported", func() {
			BeforeEach(func() {
				podAnnotations = map[string]string{
					stset.AnnotationLastReportedLRPCrash: strconv.FormatInt(timestamp.Unix(), 10),
				}
			})

			It("does not requeue", func() {
				Expect(resultErr).NotTo(HaveOccurred())
			})

			It("does not create an event", func() {
				Expect(client.CreateCallCount()).To(Equal(0))
			})
		})

		When("the crashed pod does not have a StatefulSet owner", func() {
			BeforeEach(func() {
				podOwners = podOwners[:len(podOwners)-1]
			})

			It("does not requeue", func() {
				Expect(resultErr).NotTo(HaveOccurred())
			})

			It("does not create an event", func() {
				Expect(client.CreateCallCount()).To(Equal(0))
			})
		})

		When("the associated stateful set doesn't have a LRP owner", func() {
			BeforeEach(func() {
				statefulSet = appsv1.StatefulSet{}
			})

			It("does not requeue", func() {
				Expect(resultErr).NotTo(HaveOccurred())
			})

			It("does not create an event", func() {
				Expect(client.CreateCallCount()).To(Equal(0))
			})
		})

		When("the statefulset getter fails to get", func() {
			BeforeEach(func() {
				getStSetError = errors.New("boom")
			})

			It("requeues the request", func() {
				Expect(resultErr).To(HaveOccurred())
			})

			It("does not create an event", func() {
				Expect(client.CreateCallCount()).To(Equal(0))
			})
		})

		When("creating the event errors", func() {
			BeforeEach(func() {
				client.CreateReturns(errors.New("boom"))
			})

			It("requeues the request", func() {
				Expect(resultErr).To(MatchError(ContainSubstring("failed to create event")))
			})
		})
	})

	When("a crash has occurred multiple times", func() {
		var (
			timestampFirst  time.Time
			timestampSecond time.Time
			eventTime       metav1.MicroTime
		)

		BeforeEach(func() {
			timestampFirst = time.Unix(time.Now().Unix(), 0)
			timestampSecond = timestampFirst.Add(10 * time.Second)

			crashEventGenerator.GenerateReturns(&reconciler.CrashEvent{
				ProcessGUID:    "process-guid",
				Instance:       "instance-name",
				Index:          3,
				Reason:         "Error",
				ExitCode:       6,
				CrashTimestamp: timestampSecond.Unix(),
			})

			eventTime = metav1.MicroTime{Time: timestampFirst.Add(time.Second)}

			client.ListStub = func(_ context.Context, list k8sclient.ObjectList, _ ...k8sclient.ListOption) error {
				eventList, ok := list.(*corev1.EventList)
				Expect(ok).To(BeTrue())
				eventList.Items = append(eventList.Items, corev1.Event{
					ObjectMeta:     metav1.ObjectMeta{Name: "instance-name", Namespace: "some-ns"},
					Count:          1,
					Reason:         "Container: Error",
					Message:        "Container terminated with exit code: 6",
					FirstTimestamp: metav1.Time{Time: timestampFirst},
					LastTimestamp:  metav1.Time{Time: timestampFirst},
					EventTime:      eventTime,
				})

				return nil
			}
		})

		It("looks up the event with correct list options", func() {
			_, _, actualListsOpts := client.ListArgsForCall(0)
			Expect(actualListsOpts).To(ConsistOf(
				k8sclient.MatchingLabels{
					"korifi.cloudfoundry.org/instance-index": strconv.Itoa(3),
				},
				k8sclient.InNamespace("some-ns"),
				k8sclient.MatchingFields{
					reconciler.IndexEventInvolvedObjectName: "parent-lrp",
				},
				k8sclient.MatchingFields{
					reconciler.IndexEventInvolvedObjectKind: "LRP",
				},
				k8sclient.MatchingFields{
					reconciler.IndexEventReason: "Container: Error",
				},
			))
		})

		It("updates the existing event", func() {
			Expect(client.UpdateCallCount()).To(Equal(1))
			_, object, _ := client.UpdateArgsForCall(0)

			event, ok := object.(*corev1.Event)
			Expect(ok).To(BeTrue())

			Expect(event.Namespace).To(Equal("some-ns"))
			Expect(event.Reason).To(Equal("Container: Error"))
			Expect(event.Message).To(Equal("Container terminated with exit code: 6"))
			Expect(event.Count).To(BeNumerically("==", 2))
			Expect(event.FirstTimestamp).To(Equal(metav1.NewTime(timestampFirst)))
			Expect(event.LastTimestamp).To(Equal(metav1.NewTime(timestampSecond)))
			Expect(event.EventTime).To(Equal(eventTime))
		})

		When("listing events errors", func() {
			BeforeEach(func() {
				client.ListReturns(errors.New("oof"))
			})

			It("does not create an event", func() {
				Expect(client.CreateCallCount()).To(Equal(0))
			})

			It("requeues the request", func() {
				Expect(resultErr).To(HaveOccurred())
			})
		})

		When("updating the event errors", func() {
			BeforeEach(func() {
				client.UpdateReturns(errors.New("oof"))
			})

			It("requeues the request", func() {
				Expect(resultErr).To(HaveOccurred())
			})
		})
	})

	When("getting the pod errors", func() {
		BeforeEach(func() {
			podGetError = errors.New("boom")
		})

		It("does not create an event", func() {
			Expect(client.CreateCallCount()).To(Equal(0))
		})

		It("requeues the request", func() {
			Expect(resultErr).To(HaveOccurred())
		})

		When("it returns a not found error", func() {
			BeforeEach(func() {
				podGetError = apierrors.NewNotFound(schema.GroupResource{}, "my-pod")
			})

			It("does not create an event", func() {
				Expect(client.CreateCallCount()).To(Equal(0))
			})

			It("does not requeue the request", func() {
				Expect(resultErr).NotTo(HaveOccurred())
			})
		})
	})
})
