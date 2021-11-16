package repositories

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=v1,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=v1,resources=pods/status,verbs=get

const (
	workloadsContainerName = "opi"
	cfInstanceIndexKey     = "CF_INSTANCE_INDEX"
	RunningState           = "RUNNING"
	// All below statuses changed to "DOWN" until we decide what statuses we want to support in the future
	pendingState = "STARTING"
	crashedState = "DOWN"
	unknownState = "DOWN"
)

type PodStatsRecord struct {
	Type  string
	Index int
	State string `default:"DOWN"`
}

type FetchPodStatsMessage struct {
	Namespace   string
	AppGUID     string
	Instances   int
	ProcessType string
}

type PodRepo struct{}

const (
	AppGUIDKey = "cloudfoundry.org/app_guid" // TODO: Eirini currently uses this key - get them to update this?
)

func (r *PodRepo) FetchPodStatsByAppGUID(ctx context.Context, k8sClient client.Client, message FetchPodStatsMessage) ([]PodStatsRecord, error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{AppGUIDKey: message.AppGUID})
	if err != nil {
		return nil, err
	}
	listOpts := &client.ListOptions{Namespace: message.Namespace, LabelSelector: labelSelector}

	pods, err := FetchPods(ctx, k8sClient, *listOpts)
	if err != nil {
		return nil, err
	}

	//Initialize records slice with the pod instances we expect to exist
	records := make([]PodStatsRecord, message.Instances)
	for i := 0; i < message.Instances; i++ {
		records[i] = PodStatsRecord{
			Type:  message.ProcessType,
			Index: i,
			State: unknownState,
		}
	}

	for _, p := range pods {
		index, err := extractIndex(p)
		if err != nil {
			return nil, err
		}
		setPodState(&records[index], p)
	}
	return records, nil
}

func FetchPods(ctx context.Context, k8sClient client.Client, listOpts client.ListOptions) ([]corev1.Pod, error) {
	podList := corev1.PodList{}
	err := k8sClient.List(ctx, &podList, &listOpts)
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func setPodState(record *PodStatsRecord, pod corev1.Pod) {
	record.State = getPodState(pod)
}

func extractProcessContainer(containers []corev1.Container) (corev1.Container, error) {
	for i, c := range containers {
		if c.Name == workloadsContainerName {
			return containers[i], nil
		}
	}

	return corev1.Container{}, fmt.Errorf("Could not find '%s' container", workloadsContainerName)
}

func extractIndexFromContainer(container corev1.Container) string {
	envs := container.Env
	for _, e := range envs {
		if e.Name == cfInstanceIndexKey {
			return e.Value
		}
	}
	return "-1"
}

func extractIndex(pod corev1.Pod) (int, error) {
	container, err := extractProcessContainer(pod.Spec.Containers)
	if err != nil {
		return -1, err
	}
	indexString := extractIndexFromContainer(container)
	index, err := strconv.Atoi(indexString)
	if err != nil {
		return -1, err
	}
	return index, nil
}

func getPodState(pod corev1.Pod) string {
	if len(pod.Status.ContainerStatuses) == 0 || pod.Status.Phase == corev1.PodUnknown {
		return unknownState
	}

	if podPending(&pod) {
		if containersHaveBrokenImage(pod.Status.ContainerStatuses) {
			return crashedState
		}

		return pendingState
	}

	if podFailed(&pod) {
		return crashedState
	}

	if podRunning(&pod) {
		if containersReady(pod.Status.ContainerStatuses) {
			return RunningState
		}

		if containersRunning(pod.Status.ContainerStatuses) {
			return pendingState
		}
	}

	if containersFailed(pod.Status.ContainerStatuses) {
		return crashedState
	}

	return unknownState
}

func podPending(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodPending
}

func podFailed(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed
}

func podRunning(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

func containersHaveBrokenImage(statuses []corev1.ContainerStatus) bool {
	for _, status := range statuses {
		if status.State.Waiting == nil {
			continue
		}

		if status.State.Waiting.Reason == "ErrImagePull" || status.State.Waiting.Reason == "ImagePullBackOff" {
			return true
		}
	}

	return false
}

func containersFailed(statuses []corev1.ContainerStatus) bool {
	for _, status := range statuses {
		if status.State.Waiting != nil || status.State.Terminated != nil {
			return true
		}
	}

	return false
}

func containersReady(statuses []corev1.ContainerStatus) bool {
	for _, status := range statuses {
		if status.State.Running == nil || !status.Ready {
			return false
		}
	}

	return true
}

func containersRunning(statuses []corev1.ContainerStatus) bool {
	for _, status := range statuses {
		if status.State.Running == nil {
			return false
		}
	}

	return true
}
