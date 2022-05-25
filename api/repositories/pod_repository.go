package repositories

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups="metrics.k8s.io",resources=pods,verbs=get

const (
	workloadsContainerName = "opi"
	cfInstanceIndexKey     = "CF_INSTANCE_INDEX"
	eiriniLabelVersionKey  = "korifi.cloudfoundry.org/version"
	cfProcessGuidKey       = "korifi.cloudfoundry.org/guid"
	RunningState           = "RUNNING"
	pendingState           = "STARTING"
	// All below statuses changed to "DOWN" until we decide what statuses we want to support in the future
	crashedState             = "DOWN"
	unknownState             = "DOWN"
	ProcessStatsResourceType = "Process Stats"
	PodMetricsResourceType   = "Pod Metrics"
)

type PodRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	metricsFetcher    MetricsFetcherFn
}

//counterfeiter:generate -o fake -fake-name MetricsFetcherFn . MetricsFetcherFn
type MetricsFetcherFn func(ctx context.Context, namespace, name string) (*metricsv1beta1.PodMetrics, error)

func NewPodRepo(
	userClientFactory authorization.UserK8sClientFactory,
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
		v1alpha1.CFAppGUIDLabelKey: message.AppGUID,
		eiriniLabelVersionKey:      message.AppRevision,
		cfProcessGuidKey:           message.ProcessGUID,
	})
	if err != nil {
		return nil, err
	}
	listOpts := &client.ListOptions{Namespace: message.Namespace, LabelSelector: labelSelector}

	pods, err := r.ListPods(ctx, authInfo, *listOpts)
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

		podState := getPodState(p)
		if podState == "DOWN" {
			continue
		}
		records[index].State = podState

		podMetrics, err := r.metricsFetcher(ctx, p.Namespace, p.Name)
		if err != nil {
			errorMsg := err.Error()
			if strings.Contains(errorMsg, "not found") ||
				strings.Contains(errorMsg, "the server could not find the requested resource") {
				continue
			}
			return nil, err
		}
		metricsMap := aggregateContainerMetrics(podMetrics.Containers)
		if len(metricsMap) == 0 {
			continue
		}

		if CPUquantity, ok := metricsMap["cpu"]; ok {
			value := float64(CPUquantity.ScaledValue(resource.Nano))
			percentage := value / 1e7
			records[index].Usage.CPU = &percentage
		}

		if memQuantity, ok := metricsMap["memory"]; ok {
			value := memQuantity.Value()
			records[index].Usage.Mem = &value
		}

		if storageQuantity, ok := metricsMap["storage"]; ok {
			value := storageQuantity.Value()
			records[index].Usage.Disk = &value
		}
		time := podMetrics.Timestamp.UTC().Format(TimestampFormat)
		records[index].Usage.Time = &time

	}
	return records, nil
}

func (r *PodRepo) ListPods(ctx context.Context, authInfo authorization.Info, listOpts client.ListOptions) ([]corev1.Pod, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, &listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, ProcessStatsResourceType))
	}

	return podList.Items, nil
}

func extractProcessContainer(containers []corev1.Container) (*corev1.Container, error) {
	for i, c := range containers {
		if c.Name == workloadsContainerName {
			return &containers[i], nil
		}
	}
	return nil, fmt.Errorf("container %q not found", workloadsContainerName)
}

func extractEnvVarFromContainer(container corev1.Container, envVar string) (string, error) {
	envs := container.Env
	for _, e := range envs {
		if e.Name == envVar {
			return e.Value, nil
		}
	}
	return "", fmt.Errorf("%s not set", envVar)
}

func extractIndex(pod corev1.Pod) (int, error) {
	container, err := extractProcessContainer(pod.Spec.Containers)
	if err != nil {
		return 0, err
	}

	indexString, err := extractEnvVarFromContainer(*container, cfInstanceIndexKey)
	if err != nil {
		return 0, err
	}

	index, err := strconv.Atoi(indexString)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid index: %w", cfInstanceIndexKey, err)
	}

	if index < 0 {
		return 0, fmt.Errorf("%s is not a valid index: instance indexes can't be negative", cfInstanceIndexKey)
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

func CreateMetricsFetcher(k8sClientConfig *rest.Config) (MetricsFetcherFn, error) {
	c, err := versioned.NewForConfig(k8sClientConfig)
	if err != nil {
		return nil, apierrors.FromK8sError(err, PodMetricsResourceType)
	}

	return func(ctx context.Context, namespace, name string) (*metricsv1beta1.PodMetrics, error) {
		podMetrics, err := c.MetricsV1beta1().PodMetricses(namespace).Get(ctx, name, v1.GetOptions{})
		return podMetrics, apierrors.FromK8sError(err, PodMetricsResourceType)
	}, nil
}

func aggregateContainerMetrics(containers []metricsv1beta1.ContainerMetrics) map[string]resource.Quantity {
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

type RuntimeLogsMessage struct {
	SpaceGUID   string
	AppGUID     string
	AppRevision string
	Limit       int64
}

func (r *PodRepo) GetRuntimeLogsForApp(ctx context.Context, logger logr.Logger, authInfo authorization.Info, message RuntimeLogsMessage) ([]LogRecord, error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		v1alpha1.CFAppGUIDLabelKey:        message.AppGUID,
		"korifi.cloudfoundry.org/version": message.AppRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build labelSelector: %w", err)
	}
	listOpts := client.ListOptions{Namespace: message.SpaceGUID, LabelSelector: labelSelector}

	pods, err := r.ListPods(ctx, authInfo, listOpts)
	if err != nil {
		return nil, err
	}

	appLogs := make([]LogRecord, 0, int64(len(pods))*message.Limit/2)

	k8sClient, err := r.userClientFactory.BuildK8sClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	for _, pod := range pods {
		var logReadCloser io.ReadCloser
		logReadCloser, err = k8sClient.CoreV1().Pods(message.SpaceGUID).GetLogs(pod.Name, &corev1.PodLogOptions{Timestamps: true, TailLines: &message.Limit}).Stream(ctx)
		if err != nil {
			// untested
			logger.Error(err, fmt.Sprintf("failed to fetch logs for pod: %s", pod.Name))
			continue
		}

		r := bufio.NewReader(logReadCloser)
		for {
			var line []byte
			line, err = r.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				} else {
					_ = logReadCloser.Close()
					return nil, fmt.Errorf("failed to parse pod logs: %w", err)
				}
			}

			logLine := string(line)
			var logTime int64
			logLine, logTime, _ = parseRFC3339NanoTime(logLine)

			logRecord := LogRecord{
				Message:   logLine,
				Timestamp: logTime,
			}

			appLogs = append(appLogs, logRecord)
		}

		_ = logReadCloser.Close()
	}

	return appLogs, nil
}

func parseRFC3339NanoTime(input string) (string, int64, error) {
	if len(input) < 30 {
		return input, 0, fmt.Errorf("string not long enough")
	}

	t, err := time.Parse(time.RFC3339Nano, input[:30])
	if err != nil {
		return input, 0, err
	}

	return input[31:], t.UnixNano(), nil
}
