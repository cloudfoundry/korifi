package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
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

// LogCacheHandler implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workfloh.handlerWrapper.
type LogCacheHandler struct {
	appRepo       CFAppRepository
	buildRepo     CFBuildRepository
	appLogsReader AppLogsReader
}

func NewLogCacheHandler(
	appRepo CFAppRepository,
	buildRepository CFBuildRepository,
	appLogsReader AppLogsReader,
) *LogCacheHandler {
	return &LogCacheHandler{
		appRepo:       appRepo,
		buildRepo:     buildRepository,
		appLogsReader: appLogsReader,
	}
}

func (h *LogCacheHandler) logCacheInfoHandler(r *http.Request) (*routing.Response, error) {
	return routing.NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
		"version":   logCacheVersion,
		"vm_uptime": "0",
	}), nil
}

func (h *LogCacheHandler) logCacheReadHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("log-cache-handler.log-cache-read")

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

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForLogs(logs)), nil
}

func (h *LogCacheHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", LogCacheInfoPath, routing.Handler(h.logCacheInfoHandler))
	router.Method("GET", LogCacheReadPath, routing.Handler(h.logCacheReadHandler))
}
