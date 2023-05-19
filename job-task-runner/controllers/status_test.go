package controllers_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/job-task-runner/controllers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("StatusGetter", func() {
	var (
		statusGetter  *controllers.StatusGetter
		job           *batchv1.Job
		conditions    []metav1.Condition
		conditionsErr error
	)

	BeforeEach(func() {
		job = &batchv1.Job{
			Status: batchv1.JobStatus{},
		}

		statusGetter = controllers.NewStatusGetter(ctrl.Log.WithName("test"), fakeClient)
	})

	JustBeforeEach(func() {
		conditions, conditionsErr = statusGetter.GetStatusConditions(context.Background(), job)
	})

	It("succeeds", func() {
		Expect(conditionsErr).NotTo(HaveOccurred())
	})

	It("returns an initialized condition", func() {
		initializedStatusCondition := meta.FindStatusCondition(conditions, korifiv1alpha1.TaskInitializedConditionType)
		Expect(initializedStatusCondition).NotTo(BeNil())
		Expect(initializedStatusCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(initializedStatusCondition.Reason).To(Equal("JobCreated"))
	})

	When("the job is running", func() {
		var now metav1.Time

		BeforeEach(func() {
			now = metav1.Now()
			job = &batchv1.Job{
				Status: batchv1.JobStatus{
					StartTime: &now,
				},
			}
		})

		It("contains a started condition with a matching timestamp", func() {
			startedStatusCondition := meta.FindStatusCondition(conditions, korifiv1alpha1.TaskStartedConditionType)
			Expect(startedStatusCondition).NotTo(BeNil())
			Expect(startedStatusCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(startedStatusCondition.Reason).To(Equal("JobStarted"))
			Expect(startedStatusCondition.LastTransitionTime).To(Equal(now))
		})
	})

	When("the job has succeeded", func() {
		var (
			now   metav1.Time
			later metav1.Time
		)

		BeforeEach(func() {
			now = metav1.Now()
			later = metav1.NewTime(now.Add(time.Hour))
			job = &batchv1.Job{
				Status: batchv1.JobStatus{
					StartTime:      &now,
					Succeeded:      1,
					CompletionTime: &later,
				},
			}
		})

		It("contains a succeeded condition", func() {
			succeededStatusCondition := meta.FindStatusCondition(conditions, korifiv1alpha1.TaskSucceededConditionType)
			Expect(succeededStatusCondition).NotTo(BeNil())
			Expect(succeededStatusCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(succeededStatusCondition.Reason).To(Equal("JobSucceeded"))
			Expect(succeededStatusCondition.LastTransitionTime).To(Equal(later))
		})
	})

	When("the job has failed", func() {
		var (
			now     metav1.Time
			later   metav1.Time
			podList corev1.PodList
		)

		BeforeEach(func() {
			now = metav1.Now()
			later = metav1.NewTime(now.Add(time.Hour))
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-job",
					Namespace: "my-ns",
				},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:               batchv1.JobComplete,
							LastTransitionTime: metav1.Now(),
						},
						{
							Type:               batchv1.JobFailed,
							LastTransitionTime: metav1.Now(),
						},
						{
							Type:               batchv1.JobFailed,
							LastTransitionTime: later,
						},
					},
					StartTime: &now,
					Failed:    1,
				},
			}

			podList = corev1.PodList{
				Items: []corev1.Pod{
					{
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "henry",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 1,
											Reason:   "Error",
										},
									},
								},
								{
									Name: "workload",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 42,
											Reason:   "Error",
										},
									},
								},
							},
						},
					},
				},
			}

			fakeClient.ListStub = func(ctx context.Context, objList client.ObjectList, opts ...client.ListOption) error {
				list, ok := objList.(*corev1.PodList)
				Expect(ok).To(BeTrue())
				*list = podList

				return nil
			}
		})

		It("returns a failed status with values from the failed container", func() {
			Expect(meta.IsStatusConditionTrue(conditions, korifiv1alpha1.TaskFailedConditionType)).To(BeTrue())
			failedCondition := meta.FindStatusCondition(conditions, korifiv1alpha1.TaskFailedConditionType)
			Expect(failedCondition.LastTransitionTime).To(Equal(later))
			Expect(failedCondition.Reason).To(Equal("Error"))

			Expect(fakeClient.ListCallCount()).To(Equal(1))
			_, listObj, opts := fakeClient.ListArgsForCall(0)
			Expect(listObj).To(BeAssignableToTypeOf(&corev1.PodList{}))
			Expect(opts).To(ContainElement(client.InNamespace("my-ns")))
			Expect(opts).To(ContainElement(client.MatchingLabels{"job-name": "my-job"}))

			Expect(failedCondition.Message).To(Equal("Failed with exit code: 42"))
		})

		When("listing the job pods fails", func() {
			BeforeEach(func() {
				fakeClient.ListReturns(errors.New("boom"))
			})

			It("returns the error", func() {
				Expect(conditionsErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("we get more than one pod for a job", func() {
			BeforeEach(func() {
				podList.Items = append(podList.Items, podList.Items[0])
			})

			It("returns an error", func() {
				Expect(conditionsErr).To(MatchError(ContainSubstring("found more than one pod for job")))
			})
		})

		When("there are no pods for the job", func() {
			BeforeEach(func() {
				podList.Items = nil
			})

			It("returns an error", func() {
				Expect(conditionsErr).To(MatchError(ContainSubstring("no pods found for job")))
			})
		})

		When("task container does not have termination status", func() {
			BeforeEach(func() {
				podList.Items[0].Status.ContainerStatuses[1].State.Terminated = nil
			})

			It("returns an error", func() {
				Expect(conditionsErr).To(MatchError(ContainSubstring("no terminated state found")))
			})
		})

		When("task container is not found", func() {
			BeforeEach(func() {
				podList.Items[0].Status.ContainerStatuses[1].Name = "foo"
			})

			It("returns an error", func() {
				Expect(conditionsErr).To(MatchError(ContainSubstring("no workload container found")))
			})
		})
	})
})
