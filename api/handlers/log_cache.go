package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	LogCacheInfoPath = "/api/v1/info"
	LogCacheReadPath = "/api/v1/read/{guid}"
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
}

func NewLogCache(
	requestValidator RequestValidator,
	appRepo CFAppRepository,
	buildRepository CFBuildRepository,
	logRepo LogRepository,
) *LogCache {
	return &LogCache{
		requestValidator: requestValidator,
		appRepo:          appRepo,
		buildRepo:        buildRepository,
		logRepo:          logRepo,
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

	payload := payloads.LogRead{}
	if err := h.requestValidator.DecodeAndValidateURLValues(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	appGUID := routing.URLParam(r, "guid")

	logs, err := h.getAppLogs(r.Context(), logger, authInfo, appGUID, payload)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get app logs", "app", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForLogs(logs)), nil
}

func (h *LogCache) getAppLogs(ctx context.Context, logger logr.Logger, authInfo authorization.Info, appGUID string, payload payloads.LogRead) ([]repositories.LogRecord, error) {
	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	build, err := h.buildRepo.GetLatestBuildByAppGUID(ctx, authInfo, app.SpaceGUID, app.GUID)
	if err != nil {
		if !errors.As(err, new(apierrors.NotFoundError)) {
			return nil, fmt.Errorf("failed to get latest app build: %w", err)
		}
		return []repositories.LogRecord{}, nil
	}

	logs, err := h.logRepo.GetAppLogs(ctx, authInfo, repositories.GetLogsMessage{
		App:        app,
		Build:      build,
		StartTime:  payload.StartTime,
		Limit:      payload.Limit,
		Descending: payload.Descending,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get app logs", "app", appGUID, "build", build.GUID)
	}

	return logs, nil
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
