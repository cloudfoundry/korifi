package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/go-logr/logr"
)

const (
	LogCacheInfoPath = "/api/v1/info"
	LogCacheReadPath = "/api/v1/read/{source-id}"
	logCacheVersion  = "2.11.4+cf-k8s"
)

//counterfeiter:generate -o fake -fake-name LogRepository . LogRepository
type LogRepository interface {
	GetAppLogs(context.Context, authorization.Info, repositories.GetLogsMessage) ([]repositories.LogRecord, error)
}

// LogCache implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workfloh.handlerWrapper.
type LogCache struct {
	requestValidator RequestValidator
	appRepo          CFAppRepository
	buildRepo        CFBuildRepository
	logRepo          LogRepository
	processStats     ProcessStats
}

func NewLogCache(
	requestValidator RequestValidator,
	appRepo CFAppRepository,
	buildRepository CFBuildRepository,
	logRepo LogRepository,
	processStats ProcessStats,
) *LogCache {
	return &LogCache{
		requestValidator: requestValidator,
		appRepo:          appRepo,
		buildRepo:        buildRepository,
		logRepo:          logRepo,
		processStats:     processStats,
	}
}

func (h *LogCache) info(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(map[string]interface{}{
		"version":   logCacheVersion,
		"vm_uptime": "0",
	}), nil
}

func (h *LogCache) read(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.log-cache.read")

	payload := payloads.LogCacheRead{}
	if err := h.requestValidator.DecodeAndValidateURLValues(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	appGUID := routing.URLParam(r, "source-id")
	logger = logger.WithValues("appGUID", appGUID)

	appRecord, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get app")
	}

	if tools.EmptyOrContains(payload.EnvelopeTypes, "LOG") {
		return h.readLogs(r.Context(), authInfo, appRecord, payload)
	}

	return h.readStats(r.Context(), authInfo, appRecord)
}

func (h *LogCache) readLogs(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, payload payloads.LogCacheRead) (*routing.Response, error) {
	logger := logr.FromContextOrDiscard(ctx).WithName("handlers.log-cache.read.logs").WithValues("appGUID", appRecord.GUID)
	build, err := h.buildRepo.GetLatestBuildByAppGUID(ctx, authInfo, appRecord.SpaceGUID, appRecord.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get latest app build")
	}

	logs, err := h.logRepo.GetAppLogs(ctx, authInfo, repositories.GetLogsMessage{
		App:        appRecord,
		Build:      build,
		StartTime:  payload.StartTime,
		Limit:      payload.Limit,
		Descending: payload.Descending,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get app logs")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForLogs(logs)), nil
}

func (h *LogCache) readStats(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord) (*routing.Response, error) {
	logger := logr.FromContextOrDiscard(ctx).WithName("handlers.log-cache.read.stats").WithValues("appGUID", appRecord.GUID)
	stats, err := h.processStats.FetchAppProcessesStats(ctx, authInfo, appRecord.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to fetch app stats")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForStats(appRecord, stats)), nil
}

func (h *LogCache) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: LogCacheInfoPath, Handler: h.info},
	}
}

func (h *LogCache) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: LogCacheReadPath, Handler: h.read},
	}
}
