package repositories

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appLogSourceType = "APP"
)

type PodRepo struct {
	userClientFactory authorization.UserK8sClientFactory
}

func NewPodRepo(userClientFactory authorization.UserK8sClientFactory) *PodRepo {
	return &PodRepo{
		userClientFactory: userClientFactory,
	}
}

func (r *PodRepo) listPods(ctx context.Context, authInfo authorization.Info, listOpts client.ListOptions) ([]corev1.Pod, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, &listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	return podList.Items, nil
}

type RuntimeLogsMessage struct {
	SpaceGUID   string
	AppGUID     string
	AppRevision string
	Limit       int64
}

func (r *PodRepo) GetRuntimeLogsForApp(ctx context.Context, logger logr.Logger, authInfo authorization.Info, message RuntimeLogsMessage) ([]LogRecord, error) {
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.CFAppGUIDLabelKey:  message.AppGUID,
		"korifi.cloudfoundry.org/version": message.AppRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build labelSelector: %w", err)
	}
	listOpts := client.ListOptions{Namespace: message.SpaceGUID, LabelSelector: labelSelector}

	pods, err := r.listPods(ctx, authInfo, listOpts)
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
			logger.Info(fmt.Sprintf("failed to fetch logs for pod: %s", pod.Name), "err", err)
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

			logRecord := lineToAppLogRecord(line)

			appLogs = append(appLogs, logRecord)
		}

		_ = logReadCloser.Close()
	}

	return appLogs, nil
}

func lineToAppLogRecord(line []byte) LogRecord {
	logLine := string(line)
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
