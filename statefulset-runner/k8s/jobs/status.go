package jobs

import (
	"context"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/lager"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatusGetter struct {
	logger lager.Logger
}

func NewStatusGetter(logger lager.Logger) *StatusGetter {
	return &StatusGetter{
		logger: logger,
	}
}

func (s *StatusGetter) GetStatus(ctx context.Context, job *batchv1.Job) eiriniv1.TaskStatus {
	if job.Status.StartTime == nil {
		return eiriniv1.TaskStatus{
			ExecutionStatus: eiriniv1.TaskStarting,
		}
	}

	if job.Status.Succeeded > 0 && job.Status.CompletionTime != nil {
		return eiriniv1.TaskStatus{
			ExecutionStatus: eiriniv1.TaskSucceeded,
			StartTime:       job.Status.StartTime,
			EndTime:         job.Status.CompletionTime,
		}
	}

	lastFailureTimestamp := getLastFailureTimestamp(job.Status)
	if job.Status.Failed > 0 && lastFailureTimestamp != nil {
		return eiriniv1.TaskStatus{
			ExecutionStatus: eiriniv1.TaskFailed,
			StartTime:       job.Status.StartTime,
			EndTime:         lastFailureTimestamp,
		}
	}

	return eiriniv1.TaskStatus{
		ExecutionStatus: eiriniv1.TaskRunning,
		StartTime:       job.Status.StartTime,
	}
}

func getLastFailureTimestamp(jobStatus batchv1.JobStatus) *metav1.Time {
	var lastFailure *metav1.Time

	for _, condition := range jobStatus.Conditions {
		condition := condition
		if condition.Type != batchv1.JobFailed {
			continue
		}

		if lastFailure == nil || condition.LastTransitionTime.After(lastFailure.Time) {
			lastFailure = &condition.LastTransitionTime
		}
	}

	return lastFailure
}
