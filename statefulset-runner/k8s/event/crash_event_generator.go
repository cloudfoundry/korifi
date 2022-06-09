package event

import (
	"context"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventKilling               = "Killing"
	CreateContainerConfigError = "CreateContainerConfigError"
)

type DefaultCrashEventGenerator struct {
	client client.Client
}

func NewDefaultCrashEventGenerator(client client.Client) DefaultCrashEventGenerator {
	return DefaultCrashEventGenerator{
		client: client,
	}
}

func (g DefaultCrashEventGenerator) Generate(ctx context.Context, pod *corev1.Pod, logger lager.Logger) *reconciler.CrashEvent {
	logger = logger.Session("generate-crash-event",
		lager.Data{
			"pod-name": pod.Name,
			"guid":     pod.Annotations[stset.AnnotationProcessGUID],
			"version":  pod.Annotations[stset.AnnotationVersion],
		})

	statuses := pod.Status.ContainerStatuses
	if len(statuses) == 0 {
		logger.Debug("skipping-empty-container-statuseses")

		return nil
	}

	if pod.Labels[stset.LabelSourceType] != stset.AppSourceType {
		logger.Debug("skipping-non-eirini-pod")

		return nil
	}

	appStatus := getApplicationContainerStatus(pod.Status.ContainerStatuses)
	if appStatus == nil {
		logger.Debug("skipping-eirini-pod-has-no-opi-container-statuses")

		return nil
	}

	if appStatus.State.Terminated != nil {
		return g.generateReportForTerminatedPod(ctx, pod, appStatus, logger)
	}

	if appStatus.LastTerminationState.Terminated != nil {
		reason := appStatus.LastTerminationState.Terminated.Reason
		exitCode := int(appStatus.LastTerminationState.Terminated.ExitCode)
		crashTimestamp := appStatus.LastTerminationState.Terminated.FinishedAt.Unix()

		return generateReport(pod, reason, exitCode, crashTimestamp, calculateCrashCount(appStatus))
	}

	logger.Debug("skipping-pod-healthy")

	return nil
}

func (g DefaultCrashEventGenerator) generateReportForTerminatedPod(ctx context.Context, pod *corev1.Pod, status *corev1.ContainerStatus, logger lager.Logger) *reconciler.CrashEvent {
	podEvents, err := g.getByPod(ctx, *pod)
	if err != nil {
		logger.Error("skipping-failed-to-get-k8s-events", err)

		return nil
	}

	if isStopped(podEvents) {
		logger.Debug("skipping-pod-stopped")

		return nil
	}

	terminated := status.State.Terminated

	return generateReport(pod, terminated.Reason, int(terminated.ExitCode), terminated.FinishedAt.Unix(), calculateCrashCount(status))
}

func (g DefaultCrashEventGenerator) getByPod(ctx context.Context, pod corev1.Pod) ([]corev1.Event, error) {
	eventList := &corev1.EventList{}

	err := g.client.List(ctx, eventList, client.InNamespace(pod.Namespace),
		client.MatchingFields{
			reconciler.IndexEventInvolvedObjectKind: "Pod",
		},
		client.MatchingFields{
			reconciler.IndexEventInvolvedObjectName: pod.Name,
		})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list pod events")
	}

	return eventList.Items, nil
}

func generateReport(
	pod *corev1.Pod,
	reason string,
	exitCode int,
	crashTimestamp int64,
	restartCount int,
) *reconciler.CrashEvent {
	index, _ := util.ParseAppIndex(pod.Name)

	return &reconciler.CrashEvent{
		ProcessGUID:    pod.Annotations[stset.AnnotationProcessGUID],
		Reason:         reason,
		Instance:       pod.Name,
		Index:          index,
		ExitCode:       exitCode,
		CrashTimestamp: crashTimestamp,
		CrashCount:     restartCount,
	}
}

func getApplicationContainerStatus(statuses []corev1.ContainerStatus) *corev1.ContainerStatus {
	for _, status := range statuses {
		if status.Name == stset.ApplicationContainerName {
			return &status
		}
	}

	return nil
}

// warning: apparently the RestartCount is limited to 5 by K8s Garbage
// Collection. However, we have observed it at 6 at least!

// If container is running, the restart count will be the crash count.  If
// container is terminated or waiting, we need to add 1, as it has not yet
// been restarted
func calculateCrashCount(containerState *corev1.ContainerStatus) int {
	if containerState.State.Running != nil {
		return int(containerState.RestartCount)
	}

	return int(containerState.RestartCount + 1)
}

func isStopped(events []corev1.Event) bool {
	if len(events) == 0 {
		return false
	}

	event := events[len(events)-1]

	return event.Reason == eventKilling
}
