package actions

import (
	"context"
	"sort"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"

	"github.com/go-logr/logr"
)

type AppLogs struct {
	appRepo   shared.CFAppRepository
	buildRepo shared.CFBuildRepository
	podRepo   shared.PodRepository
}

func NewAppLogs(appRepo shared.CFAppRepository, buildRepo shared.CFBuildRepository, podRepo shared.PodRepository) *AppLogs {
	return &AppLogs{
		appRepo:   appRepo,
		buildRepo: buildRepo,
		podRepo:   podRepo,
	}
}

func (a *AppLogs) Read(ctx context.Context, logger logr.Logger, authInfo authorization.Info, appGUID string, read payloads.LogRead) ([]repositories.LogRecord, error) {
	const (
		defaultLogLimit = 100
	)

	app, err := a.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	buildLogs, err := a.buildRepo.GetBuildLogs(ctx, authInfo, app.SpaceGUID, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch build logs", "AppGUID", appGUID)
	}

	logLimit := int64(defaultLogLimit)
	if read.Limit != 0 {
		logLimit = read.Limit
	}

	runtimeLogs, err := a.podRepo.GetRuntimeLogsForApp(ctx, logger, authInfo, repositories.RuntimeLogsMessage{
		SpaceGUID:   app.SpaceGUID,
		AppGUID:     app.GUID,
		AppRevision: app.Revision,
		Limit:       logLimit,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app runtime logs from Kubernetes", "AppGUID", appGUID)
	}

	logs := append(buildLogs, runtimeLogs...)

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp < logs[j].Timestamp
	})

	// ensure that we didn't exceed the log limit
	if read.Limit != 0 && int64(len(logs)) > read.Limit {
		first := int64(len(logs)) - read.Limit
		logs = logs[first:]
	}

	// filter any entries from before the start time
	if read.StartTime != 0 {
		first := sort.Search(len(logs), func(i int) bool { return read.StartTime <= logs[i].Timestamp })
		logs = logs[first:]
	}

	if read.Descending {
		for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
			logs[i], logs[j] = logs[j], logs[i]
		}
	}

	return logs, nil
}
