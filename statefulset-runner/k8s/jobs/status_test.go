package jobs_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/jobs"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("StatusGetter", func() {
	var (
		statusGetter *jobs.StatusGetter
		job          *batchv1.Job
		status       eiriniv1.TaskStatus
	)

	BeforeEach(func() {
		job = &batchv1.Job{
			Status: batchv1.JobStatus{},
		}

		statusGetter = jobs.NewStatusGetter(tests.NewTestLogger("status_getter_test"))
	})

	JustBeforeEach(func() {
		status = statusGetter.GetStatus(context.Background(), job)
	})

	It("gets the task status", func() {
		Expect(status).To(Equal(eiriniv1.TaskStatus{ExecutionStatus: eiriniv1.TaskStarting}))
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

		It("returns a running status", func() {
			Expect(status).To(Equal(eiriniv1.TaskStatus{
				ExecutionStatus: eiriniv1.TaskRunning,
				StartTime:       &now,
			}))
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

		It("returns a succeeded status", func() {
			Expect(status).To(Equal(eiriniv1.TaskStatus{
				ExecutionStatus: eiriniv1.TaskSucceeded,
				StartTime:       &now,
				EndTime:         &later,
			}))
		})
	})

	When("the job has failed", func() {
		var (
			now   metav1.Time
			later metav1.Time
		)

		BeforeEach(func() {
			now = metav1.Now()
			later = metav1.NewTime(now.Add(time.Hour))
			job = &batchv1.Job{
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
		})

		It("returns a failed status", func() {
			Expect(status).To(Equal(eiriniv1.TaskStatus{
				ExecutionStatus: eiriniv1.TaskFailed,
				StartTime:       &now,
				EndTime:         &later,
			}))
		})
	})
})
