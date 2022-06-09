package event_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/event"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/k8sfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var crashTime = meta.Time{Time: time.Now()}

var _ = Describe("CrashEventGenerator", func() {
	var (
		client    *k8sfakes.FakeClient
		logger    *tests.TestLogger
		pod       *corev1.Pod
		generator event.DefaultCrashEventGenerator
	)

	BeforeEach(func() {
		logger = tests.NewTestLogger("crash-event-logger-test")
		client = new(k8sfakes.FakeClient)
		generator = event.NewDefaultCrashEventGenerator(client)
	})

	When("app has been terminated", func() {
		BeforeEach(func() {
			pod = newTerminatedPod()
		})

		It("should generate a crashed report", func() {
			report := generator.Generate(ctx, pod, logger)
			Expect(report).To(PointTo(Equal(reconciler.CrashEvent{
				ProcessGUID:    "test-pod-anno",
				Reason:         "better luck next time",
				Instance:       "test-pod-0",
				Index:          0,
				ExitCode:       0,
				CrashCount:     9,
				CrashTimestamp: crashTime.Time.Unix(),
			})))
		})

		It("looks for events for that pod", func() {
			generator.Generate(ctx, pod, logger)

			Expect(client.ListCallCount()).To(Equal(1))
			_, _, listOptions := client.ListArgsForCall(0)
			Expect(listOptions).To(ConsistOf(
				k8sclient.InNamespace(pod.Namespace),
				k8sclient.MatchingFields{
					reconciler.IndexEventInvolvedObjectKind: "Pod",
				},
				k8sclient.MatchingFields{
					reconciler.IndexEventInvolvedObjectName: pod.Name,
				}))
		})

		When("a pod is not owned by eirini", func() {
			BeforeEach(func() {
				pod.Labels = map[string]string{}
			})

			It("should not generate", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})

			It("should provide a helpful log message", func() {
				generator.Generate(ctx, pod, logger)

				logs := logger.Logs()
				Expect(logs).To(HaveLen(1))
				log := logs[0]
				Expect(log.Message).To(Equal("crash-event-logger-test.generate-crash-event.skipping-non-eirini-pod"))
				Expect(log.Data).To(HaveKeyWithValue("pod-name", "test-pod-0"))
			})
		})

		When("pod is waiting, but hasn't been terminated", func() {
			BeforeEach(func() {
				pod = newPod([]corev1.ContainerStatus{
					{
						Name:         stset.ApplicationContainerName,
						RestartCount: 0,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "better luck next time",
							},
						},
					},
				})
			})

			It("should not emit a crashed event", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})
		})

		When("pod is stopped", func() {
			BeforeEach(func() {
				event := corev1.Event{
					InvolvedObject: corev1.ObjectReference{
						Namespace: "not-default",
						Name:      "pinky-pod",
					},
					Reason: "Killing",
				}
				client.ListStub = func(_ context.Context, list k8sclient.ObjectList, _ ...k8sclient.ListOption) error {
					eventList, ok := list.(*corev1.EventList)
					Expect(ok).To(BeTrue())
					eventList.Items = append(eventList.Items, event)

					return nil
				}
			})

			It("should not emit a crashed event", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})
		})

		When("pod is running", func() {
			BeforeEach(func() {
				pod = newRunningLastTerminatedPod()
			})

			It("sends a crash report", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(PointTo(Equal(reconciler.CrashEvent{
					ProcessGUID:    "test-pod-anno",
					Reason:         "better luck next time",
					Instance:       "test-pod-0",
					Index:          0,
					ExitCode:       0,
					CrashCount:     8,
					CrashTimestamp: crashTime.Time.Unix(),
				})))
			})
		})

		When("getting events fails", func() {
			BeforeEach(func() {
				client.ListReturns(errors.New("boom"))
			})

			It("should not emit a crashed event", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})

			It("should provide a helpful log message", func() {
				generator.Generate(ctx, pod, logger)
				logs := logger.Logs()
				Expect(logs).To(HaveLen(1))
				log := logs[0]
				Expect(log.Message).To(Equal("crash-event-logger-test.generate-crash-event.skipping-failed-to-get-k8s-events"))
				Expect(log.Data).To(HaveKeyWithValue("pod-name", "test-pod-0"))
				Expect(log.Data).To(HaveKeyWithValue("guid", "test-pod-anno"))
				Expect(log.Data).To(HaveKeyWithValue("version", "test-pod-version"))
			})
		})

		When("the sidecar is terminated", func() {
			BeforeEach(func() {
				pod = newTerminatedSidecarPod()
			})

			It("should not emit a crashed event", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})
		})

		When("the sidecar is waiting after termination", func() {
			BeforeEach(func() {
				pod = newPod([]corev1.ContainerStatus{
					{
						Name:         stset.ApplicationContainerName,
						RestartCount: 1,
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
					{
						Name:         "some-sidecar-container",
						RestartCount: 1,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason:  event.CreateContainerConfigError,
								Message: "not configured properly",
							},
						},
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:     "better luck next time",
								FinishedAt: crashTime,
								ExitCode:   1,
							},
						},
					},
				})
			})

			It("should not emit a crashed event", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})
		})
	})

	When("app is in waiting state after terimnation", func() {
		BeforeEach(func() {
			pod = newPod([]corev1.ContainerStatus{
				{
					Name:         stset.ApplicationContainerName,
					RestartCount: 1,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  event.CreateContainerConfigError,
							Message: "not configured properly",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "better luck next time",
							FinishedAt: crashTime,
							ExitCode:   1,
						},
					},
				},
			})
		})

		It("should return a crashed report", func() {
			report := generator.Generate(ctx, pod, logger)
			Expect(report).To(PointTo(Equal(reconciler.CrashEvent{
				ProcessGUID:    "test-pod-anno",
				Reason:         "better luck next time",
				Instance:       "test-pod-0",
				ExitCode:       1,
				CrashCount:     2,
				CrashTimestamp: crashTime.Unix(),
			})))
		})
	})

	When("a pod has no container statuses", func() {
		BeforeEach(func() {
			pod = newTerminatedPod()
		})

		When("the container statuses is nil", func() {
			BeforeEach(func() {
				pod.Status.ContainerStatuses = nil
			})

			It("should not send any reports", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})
		})

		When("the container statuses is empty", func() {
			BeforeEach(func() {
				pod.Status.ContainerStatuses = []corev1.ContainerStatus{}
			})

			It("should not send any reports", func() {
				report := generator.Generate(ctx, pod, logger)
				Expect(report).To(BeNil())
			})
		})
	})

	When("a pod has no application container statuses", func() {
		BeforeEach(func() {
			pod = newPod([]corev1.ContainerStatus{
				{
					Name:         "some-other-container",
					RestartCount: 1,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "better luck next time",
							FinishedAt: crashTime,
							ExitCode:   1,
						},
					},
				},
			})
		})

		It("should not send any reports", func() {
			report := generator.Generate(ctx, pod, logger)
			Expect(report).To(BeNil())
		})

		It("should provide a helpful log message", func() {
			generator.Generate(ctx, pod, logger)
			logs := logger.Logs()
			Expect(logs).To(HaveLen(1))
			log := logs[0]
			Expect(log.Message).To(Equal("crash-event-logger-test.generate-crash-event.skipping-eirini-pod-has-no-opi-container-statuses"))
			Expect(log.Data).To(HaveKeyWithValue("pod-name", "test-pod-0"))
			Expect(log.Data).To(HaveKeyWithValue("guid", "test-pod-anno"))
			Expect(log.Data).To(HaveKeyWithValue("version", "test-pod-version"))
		})
	})
})

func newTerminatedPod() *corev1.Pod {
	return newPod([]corev1.ContainerStatus{
		{
			Name:         "some-sidecar-container",
			RestartCount: 1,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
		},
		{
			Name:         stset.ApplicationContainerName,
			RestartCount: 8,
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					Reason:     "better luck next time",
					FinishedAt: crashTime,
					ExitCode:   0,
				},
			},
		},
	})
}

func newRunningLastTerminatedPod() *corev1.Pod {
	return newPod([]corev1.ContainerStatus{
		{
			Name:         "some-sidecar-container",
			RestartCount: 1,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
		},
		{
			Name:         stset.ApplicationContainerName,
			RestartCount: 8,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
			LastTerminationState: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					Reason:     "better luck next time",
					FinishedAt: crashTime,
					ExitCode:   0,
				},
			},
		},
	})
}

func newTerminatedSidecarPod() *corev1.Pod {
	return newPod([]corev1.ContainerStatus{
		{
			Name:         stset.ApplicationContainerName,
			RestartCount: 1,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
		},
		{
			Name:         "some-sidecar-container",
			RestartCount: 8,
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					Reason:     "better luck next time",
					FinishedAt: crashTime,
					ExitCode:   0,
				},
			},
		},
	})
}

func newPod(statuses []corev1.ContainerStatus) *corev1.Pod {
	name := "test-pod"

	return &corev1.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: fmt.Sprintf("%s-%d", name, 0),
			Labels: map[string]string{
				stset.LabelSourceType: stset.AppSourceType,
			},
			Annotations: map[string]string{
				stset.AnnotationProcessGUID: fmt.Sprintf("%s-anno", name),
				stset.AnnotationVersion:     fmt.Sprintf("%s-version", name),
			},
			OwnerReferences: []meta.OwnerReference{
				{
					Kind: "StatefulSet",
					Name: "mr-stateful",
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: statuses,
		},
	}
}
