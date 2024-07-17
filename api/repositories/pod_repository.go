package repositories

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	k8sclient "k8s.io/client-go/kubernetes"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appLogSourceType = "APP"
)

type RuntimeLogsMessage struct {
	SpaceGUID   string
	AppGUID     string
	AppRevision string
	Limit       int64
}

type PodRepo struct {
	userClientFactory authorization.UserK8sClientFactory
}

func NewPodRepo(userClientFactory authorization.UserK8sClientFactory) *PodRepo {
	return &PodRepo{
		userClientFactory: userClientFactory,
	}
}

func (r *PodRepo) DeletePod(ctx context.Context, authInfo authorization.Info, appRevision string, process ProcessRecord, instanceID string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		"korifi.cloudfoundry.org/app-guid":     process.AppGUID,
		"korifi.cloudfoundry.org/version":      appRevision,
		"korifi.cloudfoundry.org/process-type": process.Type,
	})
	if err != nil {
		return fmt.Errorf("failed to build labelSelector: %w", apierrors.FromK8sError(err, PodResourceType))
	}
	listOpts := client.ListOptions{Namespace: process.SpaceGUID, LabelSelector: labelSelector}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, &listOpts)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	podsToDelete := itx.FromSlice(podList.Items).Filter(func(pod corev1.Pod) bool {
		return strings.HasSuffix(pod.Name, instanceID)
	}).Collect()

	if len(podsToDelete) == 0 {
		return apierrors.NewNotFoundError(nil, PodResourceType)
	}

	if len(podsToDelete) > 1 {
		return apierrors.NewUnprocessableEntityError(nil, "multiple pods found")
	}

	err = userClient.Delete(ctx, &podsToDelete[0])
	if err != nil {
		return fmt.Errorf("failed to 'delete' pod: %w", apierrors.FromK8sError(err, PodResourceType))
	}
	return nil
}

func (r *PodRepo) GetRuntimeLogsForApp(ctx context.Context, logger logr.Logger, authInfo authorization.Info, message RuntimeLogsMessage) ([]LogRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.CFAppGUIDLabelKey: message.AppGUID,
		korifiv1alpha1.VersionLabelKey:   message.AppRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build labelSelector: %w", err)
	}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, &client.ListOptions{Namespace: message.SpaceGUID, LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	logClient, err := r.userClientFactory.BuildK8sClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	logRecords := itx.Map(slices.Values(podList.Items), func(pod corev1.Pod) func(func(LogRecord) bool) {
		return r.getPodLogs(ctx, logClient, logger, pod, message.Limit)
	}).Collect()

	return slices.Collect(it.Chain(logRecords...)), nil
}

func (r *PodRepo) getPodLogs(ctx context.Context, k8sClient k8sclient.Interface, logger logr.Logger, pod corev1.Pod, limit int64) itx.Iterator[LogRecord] {
	logReadCloser, err := k8sClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Timestamps: true, TailLines: &limit}).Stream(ctx)
	if err != nil {
		logger.Info("failed to fetch logs", "pod", pod.Name, "reason", err)
		return itx.Exhausted[LogRecord]()
	}

	defer logReadCloser.Close()

	logLines := readLines(logReadCloser, logger.WithValues("pod", pod.Name))
	return itx.Map(slices.Values(logLines), logLineToLogRecord)
}

func readLines(r io.Reader, logger logr.Logger) []string {
	lines := []string{}

	var err error
	var line []byte
	bufReader := bufio.NewReader(r)
	for {
		line, err = bufReader.ReadBytes('\n')
		lines = append(lines, string(line))
		if err != nil {
			if err != io.EOF {
				logger.Info("failed to parse pod logs", "err", err)
			}
			break
		}
	}

	return lines
}

func logLineToLogRecord(logLine string) LogRecord {
	var logTime int64
	logLine, logTime, _ = parseRFC3339NanoTime(logLine)

	// trim trailing newlines so that the CLI doesn't render extra log lines for them
	logLine = strings.TrimRight(logLine, "\r\n")

	logRecord := LogRecord{
		Message:   logLine,
		Timestamp: logTime,
		Tags: map[string]string{
			"source_type": appLogSourceType,
		},
	}
	return logRecord
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
