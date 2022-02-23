package repositories

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="metrics.k8s.io",resources=pods,verbs=get;list;watch

const (
	workloadsContainerName = "opi"
	cfInstanceIndexKey     = "CF_INSTANCE_INDEX"
	eiriniLabelVersionKey  = "workloads.cloudfoundry.org/version"
	cfProcessGuidKey       = "workloads.cloudfoundry.org/guid"
	RunningState           = "RUNNING"
	pendingState           = "STARTING"
	// All below statuses changed to "DOWN" until we decide what statuses we want to support in the future
	crashedState             = "DOWN"
	unknownState             = "DOWN"
	ProcessStatsResourceType = "Process Stats"
)

type PodRepo struct {
	userClientFactory UserK8sClientFactory
	metricsFetcher    MetricsFetcherFn
}

//counterfeiter:generate -o fake -fake-name MetricsFetcherFn . MetricsFetcherFn
type MetricsFetcherFn func(ctx context.Context, namespace, name string) (*metricsv1beta1.PodMetrics, error)

func NewPodRepo(
	userClientFactory UserK8sClientFactory,
	metricsFetcher MetricsFetcherFn,
) *PodRepo {
	return &PodRepo{
		userClientFactory: userClientFactory,
		metricsFetcher:    metricsFetcher,
	}
}

type PodStatsRecord struct {
	Type  string
	Index int
	State string `default:"DOWN"`
	Usage Usage
}

type Usage struct {
	Time *string
	CPU  *float64
	Mem  *int64
	Disk *int64
}

type ListPodStatsMessage struct {
	Namespace   string
	AppGUID     string
	AppRevision string
	Instances   int
	ProcessGUID string
	ProcessType string
}

func (r *PodRepo) ListPodStats(ctx context.Context, authInfo authorization.Info, message ListPodStatsMessage) ([]PodStatsRecord, error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		workloadsv1alpha1.CFAppGUIDLabelKey: message.AppGUID,
		eiriniLabelVersionKey:               message.AppRevision,
		cfProcessGuidKey:                    message.ProcessGUID,
	})
	if err != nil {
		return nil, err
	}
	listOpts := &client.ListOptions{Namespace: message.Namespace, LabelSelector: labelSelector}

	pods, err := r.listPods(ctx, authInfo, *listOpts)
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

		if records[index].State == "DOWN" {
			continue
		}

		podMetrics, err := r.metricsFetcher(ctx, p.Namespace, p.Name)
		if err != nil {
			if !strings.Contains(err.Error(), "the server could not find the requested resource") {
				return nil, err
			}
			continue
		}
		metricsMap := aggregateContainerMetrics(podMetrics.Containers)
		if len(metricsMap) == 0 {
			continue
		}

		if quantity, ok := metricsMap["cpu"]; ok {
			value := float64(quantity.ScaledValue(resource.Nano))
			percentage := value / 1e7
			records[index].Usage.CPU = &percentage
		}

		if r2, ok := metricsMap["memory"]; ok {
			value := r2.Value()
			records[index].Usage.Mem = &value
		}

		if r3, ok := metricsMap["storage"]; ok {
			value := r3.Value()
			records[index].Usage.Disk = &value
		}
		time := podMetrics.Timestamp.UTC().Format(TimestampFormat)
		records[index].Usage.Time = &time

	}
	return records, nil
}

func (r *PodRepo) listPods(ctx context.Context, authInfo authorization.Info, listOpts client.ListOptions) ([]corev1.Pod, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, &listOpts)
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return nil, NewForbiddenError(ProcessStatsResourceType, err)
		}

		return nil, fmt.Errorf("err in client.List: %w", err)
	}

	return podList.Items, nil
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

// Untested
func CreateMetricsFetcher() (MetricsFetcherFn, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	c, err := versioned.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, namespace, name string) (*v1beta1.PodMetrics, error) {
		return c.MetricsV1beta1().PodMetricses(namespace).Get(ctx, name, v1.GetOptions{})
	}, nil
}

func aggregateContainerMetrics(containers []v1beta1.ContainerMetrics) map[string]resource.Quantity {
	metrics := map[string]resource.Quantity{}

	for _, container := range containers {
		for k, v := range container.Usage {
			if value, ok := metrics[string(k)]; ok {
				value.Add(v)
				metrics[string(k)] = value
			} else {
				metrics[string(k)] = v
			}
		}
	}

	return metrics
}
