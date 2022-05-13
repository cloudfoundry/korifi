package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-logr/logr"
)

//counterfeiter:generate -o fake -fake-name ReadAppLogs . ReadAppLogsAction
type ReadAppLogsAction func(ctx context.Context, authInfo authorization.Info, appGUID string, read payloads.LogRead) ([]repositories.LogRecord, error)

type ReadAppLogs struct {
	appRepo   CFAppRepository
	buildRepo CFBuildRepository
	podRepo   PodRepository
}

func NewReadAppLogs(appRepo CFAppRepository, buildRepo CFBuildRepository, podRepo PodRepository) *ReadAppLogs {
	return &ReadAppLogs{
		appRepo:   appRepo,
		buildRepo: buildRepo,
		podRepo:   podRepo,
	}
}

func (a *ReadAppLogs) Invoke(ctx context.Context, logger logr.Logger, authInfo authorization.Info, appGUID string, read payloads.LogRead) ([]repositories.LogRecord, error) {
	const (
		defaultLogLimit = 100
	)

	app, err := a.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	build, err := a.buildRepo.GetLatestBuildByAppGUID(ctx, authInfo, app.SpaceGUID, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch latest app CFBuild from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	buildLogs, err := a.buildRepo.GetBuildLogs(ctx, authInfo, app.SpaceGUID, build.GUID)
	if err != nil {
		logger.Error(err, "Failed to fetch build logs", "AppGUID", appGUID, "BuildGUID", build.GUID)
		return nil, err
	}

	logLimit := int64(defaultLogLimit)
	if read.Limit != nil {
		logLimit = *read.Limit
	}

	runtimeLogs, err := a.podRepo.GetRuntimeLogsForApp(ctx, logger, authInfo, repositories.RuntimeLogsMessage{
		SpaceGUID:   app.SpaceGUID,
		AppGUID:     app.GUID,
		AppRevision: app.Revision,
		Limit:       logLimit,
	})
	if err != nil {
		logger.Error(err, "Failed to fetch app runtime logs from Kubernetes", "AppGUID", appGUID)
		return nil, err
	}

	return append(buildLogs, runtimeLogs...), nil
}
