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

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	"github.com/go-playground/validator"
)

const (
	LogCacheInfoPath = "/api/v1/info"
	LogCacheReadPath = "/api/v1/read/{guid}"
	logCacheVersion  = "2.11.4+cf-k8s"
)

//counterfeiter:generate -o fake -fake-name AppLogsReader . AppLogsReader
type AppLogsReader interface {
	Read(ctx context.Context, logger logr.Logger, authInfo authorization.Info, appGUID string, read payloads.LogRead) ([]repositories.LogRecord, error)
}

// LogCache implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workfloh.handlerWrapper.
type LogCache struct {
	appRepo       CFAppRepository
	buildRepo     CFBuildRepository
	appLogsReader AppLogsReader
}

func NewLogCache(
	appRepo CFAppRepository,
	buildRepository CFBuildRepository,
	appLogsReader AppLogsReader,
) *LogCache {
	return &LogCache{
		appRepo:       appRepo,
		buildRepo:     buildRepository,
		appLogsReader: appLogsReader,
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

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	logReadPayload := new(payloads.LogRead)
	err := payloads.Decode(logReadPayload, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	v := validator.New()
	if logReadPayloadErr := v.Struct(logReadPayload); logReadPayloadErr != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(logReadPayloadErr, "error validating log read query parameters"),
			"Error validating log read request query parameters",
		)
	}

	appGUID := chi.URLParam(r, "guid")

	var logs []repositories.LogRecord
	logs, err = h.appLogsReader.Read(r.Context(), logger, authInfo, appGUID, *logReadPayload)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to read app logs", "appGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForLogs(logs)), nil
}

func (h *LogCache) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", LogCacheInfoPath, routing.Handler(h.info))
	router.Method("GET", LogCacheReadPath, routing.Handler(h.read))
}
