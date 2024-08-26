package repositories

import (
	"context"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/tools"
	k8sclient "k8s.io/client-go/kubernetes"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildWorkloadLabelKey = "korifi.cloudfoundry.org/build-workload-name"
)

//counterfeiter:generate -o fake -fake-name LogStreamer . LogStreamer
type LogStreamer func(context.Context, k8sclient.Interface, corev1.Pod, corev1.PodLogOptions) (io.ReadCloser, error)

type GetLogsMessage struct {
	App   AppRecord
	Build BuildRecord

	StartTime  *int64
	Limit      *int64
	Descending bool
}

type LogRecord struct {
	Message   string
	Timestamp int64
	Header    string
	Tags      map[string]string
}

var DefaultLogStreamer LogStreamer = func(
	ctx context.Context,
	logClient k8sclient.Interface,
	pod corev1.Pod,
	logOpts corev1.PodLogOptions,
) (io.ReadCloser, error) {
	return logClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &logOpts).Stream(ctx)
}

type logRecordSortOrder func(LogRecord, LogRecord) int

var ascendingOrder logRecordSortOrder = func(r1, r2 LogRecord) int {
	return int(r1.Timestamp - r2.Timestamp)
}

var descendingOrder logRecordSortOrder = func(r1, r2 LogRecord) int {
	return int(r2.Timestamp - r1.Timestamp)
}

type LogRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	logStreamer       LogStreamer
}

func NewLogRepo(
	userClientFactory authorization.UserK8sClientFactory,
	logStreamer LogStreamer,
) *LogRepo {
	return &LogRepo{
		userClientFactory: userClientFactory,
		logStreamer:       logStreamer,
	}
}

func (r *LogRepo) GetAppLogs(ctx context.Context, authInfo authorization.Info, message GetLogsMessage) ([]LogRecord, error) {
	buildLogs, err := r.getBuildLogs(ctx, authInfo, message.Build, message.StartTime, message.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get build logs: %w", err)
	}

	appLogs, err := r.getAppLogs(ctx, authInfo, message.App, message.StartTime, message.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get app logs: %w", err)
	}

	logs := itx.From(buildLogs).Chain(appLogs).Filter(func(r LogRecord) bool {
		// Even though we have listed logs with `SinceTime` option, ensure that
		// there are no log entries several milliseconds before the StartTime
		// `SinceTime` log option has a precision of a second, therefore listed
		// logs could contain entries that are a millisecond prior the
		// `StartTime` if the `StartTime` has seconds fraction.
		// See	https://github.com/kubernetes/kubernetes/issues/77856 and
		// https://github.com/kubernetes/kubernetes/pull/92595
		if message.StartTime == nil || *message.StartTime < 0 {
			return true
		}
		return r.Timestamp >= *message.StartTime
	}).Collect()

	sortOrder := ascendingOrder
	if message.Descending {
		sortOrder = descendingOrder
	}
	slices.SortFunc(logs, sortOrder)

	if message.Limit == nil {
		return logs, nil
	}

	if len(logs) <= int(*message.Limit) {
		return logs, nil
	}

	return logs[:len(logs)-int(*message.Limit)], nil
}

func (r *LogRepo) getBuildLogs(
	ctx context.Context,
	authInfo authorization.Info,
	build BuildRecord,
	startTime *int64,
	limit *int64,
) (iter.Seq[LogRecord], error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		BuildWorkloadLabelKey: build.GUID,
	})
	if err != nil {
		return nil, err
	}

	logs, err := r.getLogs(
		ctx,
		authInfo,
		&client.ListOptions{
			Namespace:     build.SpaceGUID,
			LabelSelector: labelSelector,
		},
		startTime,
		limit,
	)
	if err != nil {
		return nil, err
	}

	return it.Map(logs, func(record LogRecord) LogRecord {
		record.Tags = map[string]string{
			"source_type": "STG",
		}
		return record
	}), nil
}

func (r *LogRepo) getAppLogs(
	ctx context.Context,
	authInfo authorization.Info,
	app AppRecord,
	startTime *int64,
	limit *int64,
) (iter.Seq[LogRecord], error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.CFAppGUIDLabelKey: app.GUID,
		korifiv1alpha1.VersionLabelKey:   app.Revision,
	})
	if err != nil {
		return nil, err
	}

	logs, err := r.getLogs(
		ctx,
		authInfo,
		&client.ListOptions{
			Namespace:     app.SpaceGUID,
			LabelSelector: labelSelector,
		},
		startTime,
		limit,
	)
	if err != nil {
		return nil, err
	}

	return it.Map(logs, func(record LogRecord) LogRecord {
		record.Tags = map[string]string{
			"source_type": "APP",
		}
		return record
	}), nil
}

func (r *LogRepo) getLogs(
	ctx context.Context,
	authInfo authorization.Info,
	podListOptions *client.ListOptions,
	startTime *int64,
	limit *int64,
) (iter.Seq[LogRecord], error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	logClient, err := r.userClientFactory.BuildK8sClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, podListOptions)
	if err != nil {
		return nil, apierrors.FromK8sError(err, PodResourceType)
	}

	podLogRecords := slices.Collect(it.Map(slices.Values(podList.Items), func(pod corev1.Pod) func(func(LogRecord) bool) {
		return r.getLogsForPod(ctx, logClient, pod, startTime, limit)
	}))

	return it.Chain(podLogRecords...), nil
}

func (r *LogRepo) getLogsForPod(ctx context.Context, k8sClient k8sclient.Interface, pod corev1.Pod, startTime *int64, limit *int64) iter.Seq[LogRecord] {
	readyContaines := getReadyContainers(pod)

	readyContainerLogs := slices.Collect(it.Map(slices.Values(readyContaines), func(containerName string) func(func(LogRecord) bool) {
		return r.getContainerLogs(ctx, k8sClient, pod, corev1.PodLogOptions{
			Container:  containerName,
			Timestamps: true,
			SinceTime:  toMetav1Time(startTime),
			TailLines:  limit,
		})
	}))

	return it.Chain(readyContainerLogs...)
}

func (r *LogRepo) getContainerLogs(ctx context.Context, k8sClient k8sclient.Interface, pod corev1.Pod, logOpts corev1.PodLogOptions) iter.Seq[LogRecord] {
	logger := logr.FromContextOrDiscard(ctx).WithName("get-container-logs").WithValues("pod", pod.Name)

	logReadCloser, err := r.logStreamer(ctx, k8sClient, pod, logOpts)
	if err != nil {
		logger.Info("failed to fetch logs", "reason", err)
		return it.Exhausted[LogRecord]()
	}

	defer logReadCloser.Close()

	logLines := itx.FromSlice(readLines(ctx, logReadCloser)).Filter(func(logLine string) bool {
		return len(logLine) > 0
	})

	return it.Map(logLines, logLineToLogRecord)
}

func getReadyContainers(pod corev1.Pod) []string {
	containerStatuses := append(slices.Clone(pod.Status.InitContainerStatuses), pod.Status.ContainerStatuses...)
	readyContainers := it.Filter(slices.Values(containerStatuses), func(status corev1.ContainerStatus) bool {
		return status.State.Waiting == nil
	})

	return slices.Collect(it.Map(readyContainers, func(container corev1.ContainerStatus) string {
		return container.Name
	}))
}

func readLines(ctx context.Context, r io.Reader) []string {
	logger := logr.FromContextOrDiscard(ctx)

	lines, err := it.TryCollect(it.LinesString(r))
	if err != nil {
		logger.Info("failed to parse pod logs", "err", err)
	}

	return lines
}

func logLineToLogRecord(logLine string) LogRecord {
	var logTime int64
	logLine, logTime = parseRFC3339NanoTime(logLine)

	// trim trailing newlines so that the CLI doesn't render extra log lines for them
	logLine = strings.TrimRight(logLine, "\r\n")

	logRecord := LogRecord{
		Message:   logLine,
		Timestamp: logTime,
	}
	return logRecord
}

func parseRFC3339NanoTime(input string) (string, int64) {
	timestampSeparatorIndex := strings.Index(input, " ")
	if timestampSeparatorIndex < 0 {
		return input, 0
	}

	t, err := time.Parse(time.RFC3339Nano, input[:timestampSeparatorIndex])
	if err != nil {
		return input, 0
	}

	return input[timestampSeparatorIndex+1:], t.UnixNano()
}

func toMetav1Time(timestamp *int64) *metav1.Time {
	if timestamp == nil {
		return nil
	}

	return tools.PtrTo(metav1.NewTime(time.Unix(0, *timestamp)))
}
