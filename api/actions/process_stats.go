package actions

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ApplicationContainerName = "application"
	LabelGUID                = "korifi.cloudfoundry.org/guid"
	stateStarting            = "STARTING"
	stateRunning             = "RUNNING"
	stateDown                = "DOWN"
	stateCrashed             = "CRASHED"
)

//counterfeiter:generate -o fake -fake-name MetricsRepository . MetricsRepository

type (
	MetricsRepository interface {
		GetMetrics(ctx context.Context, authInfo authorization.Info, namespace string, podSelector client.MatchingLabels) ([]repositories.PodMetrics, error)
	}

	Usage struct {
		Time *string
		CPU  *float64
		Mem  *int64
		Disk *int64
	}

	PodStatsRecord struct {
		Type      string
		Index     int
		State     string `default:"DOWN"`
		Usage     Usage
		MemQuota  *int64
		DiskQuota *int64
	}

	ProcessStats struct {
		processRepo shared.CFProcessRepository
		appRepo     shared.CFAppRepository
		metricsRepo MetricsRepository
	}
)

func NewProcessStats(processRepo shared.CFProcessRepository, appRepo shared.CFAppRepository, metricsRepo MetricsRepository) *ProcessStats {
	return &ProcessStats{
		processRepo: processRepo,
		appRepo:     appRepo,
		metricsRepo: metricsRepo,
	}
}

func (a *ProcessStats) FetchStats(ctx context.Context, authInfo authorization.Info, processGUID string) ([]PodStatsRecord, error) {
	processRecord, err := a.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		return nil, err
	}

	appRecord, err := a.appRepo.GetApp(ctx, authInfo, processRecord.AppGUID)
	if err != nil {
		return nil, err
	}

	if appRecord.State == repositories.StoppedState {
		return []PodStatsRecord{
			{
				Type:  processRecord.Type,
				Index: 0,
				State: "DOWN",
			},
		}, nil
	}

	metrics, err := a.metricsRepo.GetMetrics(ctx, authInfo, appRecord.SpaceGUID, client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey: appRecord.GUID,
		korifiv1alpha1.VersionLabelKey:   appRecord.Revision,
		LabelGUID:                        processGUID,
	})
	if err != nil {
		return nil, err
	}

	// Initialize records slice with the pod instances we expect to exist
	records := make([]PodStatsRecord, processRecord.DesiredInstances)
	for i := range records {
		records[i] = PodStatsRecord{
			Type:  processRecord.Type,
			Index: i,
			State: stateDown,
		}
	}

	for _, m := range metrics {
		index, err := extractIndex(m.Pod)
		if err != nil {
			return nil, err
		}

		podState := getPodState(m.Pod)
		if podState == stateDown {
			continue
		}

		if index >= len(records) {
			continue
		}

		records[index].State = podState

		metricsMap := aggregateContainerMetrics(m.Metrics.Containers)
		if len(metricsMap) == 0 {
			continue
		}

		if cpuQuantity, ok := metricsMap["cpu"]; ok {
			value := float64(cpuQuantity.ScaledValue(resource.Nano))
			// CF tracks CPU usage as a percentage of cores used.
			// Convert the number of nanoCPU to CPU for greatest accuracy.
			percentage := value / 1e9
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

		time := m.Metrics.Timestamp.UTC().Format(time.RFC3339)
		records[index].Usage.Time = &time

		records[index].MemQuota = tools.PtrTo(megabytesToBytes(processRecord.MemoryMB))
		records[index].DiskQuota = tools.PtrTo(megabytesToBytes(processRecord.DiskQuotaMB))
	}
	return records, nil
}

func extractIndex(pod corev1.Pod) (int, error) {
	indexString, exists := pod.ObjectMeta.Labels[korifiv1alpha1.PodIndexLabelKey]
	if !exists {
		return 0, fmt.Errorf("%s label not found", korifiv1alpha1.PodIndexLabelKey)
	}

	index, err := strconv.Atoi(indexString)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid index: %w", korifiv1alpha1.PodIndexLabelKey, err)
	}

	if index < 0 {
		return 0, fmt.Errorf("%s is not a valid index: instance indexes can't be negative", korifiv1alpha1.PodIndexLabelKey)
	}

	return index, nil
}

// Logic from Kubernetes in Action 2nd Edition - Ch 6.
// DOWN => !pod || !pod.conditions.PodScheduled
// CRASHED => any(pod.ContainerStatuses.State isA Terminated)
// RUNNING => pod.conditions.Ready
// STARTING => default

func getPodState(pod corev1.Pod) string {
	// return running when all containers are ready
	if podConditionStatus(pod, corev1.PodReady) {
		return stateRunning
	}

	if !podConditionStatus(pod, corev1.PodScheduled) {
		return stateDown
	}

	if podHasCrashedContainer(pod) {
		return stateCrashed
	}

	return stateStarting
}

func podHasCrashedContainer(pod corev1.Pod) bool {
	for _, cond := range pod.Status.ContainerStatuses {
		if cond.State.Waiting != nil && cond.State.Waiting.Reason == "CrashLoopBackOff" {
			return true
		}
	}

	return false
}

func podConditionStatus(pod corev1.Pod, conditionType corev1.PodConditionType) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == conditionType {
			return cond.Status == corev1.ConditionTrue
		}
	}

	return false
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

func megabytesToBytes(mb int64) int64 {
	return mb * 1024 * 1024
}
