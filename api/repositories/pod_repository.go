package repositories

import (
	"context"
	"strconv"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

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

func (r *PodRepo) FetchPodStatsByAppGUID(ctx context.Context, k8sClient client.Client, message FetchPodStatsMessage) ([]PodStatsRecord, error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{workloadsv1alpha1.CFAppGUIDLabelKey: message.AppGUID})
	if err != nil {
		return nil, err
	}
	listOpts := &client.ListOptions{Namespace: message.Namespace, LabelSelector: labelSelector}

	pods, err := FetchPods(ctx, k8sClient, *listOpts)
	if err != nil {
		return nil, err
	}

	// Initialize records slice with the pod instances we expect to exist
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
		if index >= 0 {
			setPodState(&records[index], p)
		}
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

func (r *PodRepo) WatchForPodsTermination(ctx context.Context, k8sClient client.Client, appGUID, namespace string) (bool, error) {
	err := wait.PollUntilWithContext(ctx, time.Second*1, func(ctx context.Context) (done bool, err error) {
		podList := corev1.PodList{}
		labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{workloadsv1alpha1.CFAppGUIDLabelKey: appGUID})
		if err != nil {
			return false, err
		}
		listOpts := &client.ListOptions{Namespace: namespace, LabelSelector: labelSelector}
		err = k8sClient.List(ctx, &podList, listOpts)
		if err != nil {
			return false, err
		}
		if len(podList.Items) == 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func setPodState(record *PodStatsRecord, pod corev1.Pod) {
	record.State = getPodState(pod)
}

func extractProcessContainer(containers []corev1.Container) *corev1.Container {
	for i, c := range containers {
		if c.Name == workloadsContainerName {
			return &containers[i]
		}
	}
	return nil
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
	container := extractProcessContainer(pod.Spec.Containers)
	if container == nil {
		return -1, nil
	}
	indexString := extractIndexFromContainer(*container)
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
