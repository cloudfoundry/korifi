package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/go-playground/validator"
	"github.com/gorilla/mux"
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
	authenticatedHandlerWrapper   *AuthAwareHandlerFuncWrapper
	unauthenticatedHandlerWrapper *AuthAwareHandlerFuncWrapper
	appRepo                       CFAppRepository
	buildRepo                     CFBuildRepository
	appLogsReader                 AppLogsReader
}

func NewLogCacheHandler(
	appRepo CFAppRepository,
	buildRepository CFBuildRepository,
	appLogsReader AppLogsReader,
) *LogCacheHandler {
	return &LogCacheHandler{
		authenticatedHandlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("LogCacheHandler")),
		unauthenticatedHandlerWrapper: NewUnauthenticatedHandlerFuncWrapper(ctrl.Log.WithName("LogCacheHandler")),
		appRepo:                       appRepo,
		buildRepo:                     buildRepository,
		appLogsReader:                 appLogsReader,
	}
}

func (h *LogCacheHandler) logCacheInfoHandler(ctx context.Context, logger logr.Logger, _ authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
		"version":   logCacheVersion,
		"vm_uptime": "0",
	}), nil
}

func (h *LogCacheHandler) logCacheReadHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	logReadPayload := new(payloads.LogRead)
	err := payloads.Decode(logReadPayload, r.Form)
	if err != nil {
		logger.Error(err, "Unable to decode request query parameters")
		return nil, err
	}

	v := validator.New()
	if logReadPayloadErr := v.Struct(logReadPayload); logReadPayloadErr != nil {
		logger.Error(logReadPayloadErr, "Error validating log read request query parameters")
		return nil, apierrors.NewUnprocessableEntityError(logReadPayloadErr, "error validating log read query parameters")
	}

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var logs []repositories.LogRecord
	logs, err = h.appLogsReader.Read(ctx, logger, authInfo, appGUID, *logReadPayload)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to read app logs", "appGUID", appGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForLogs(logs)), nil
}

func (h *LogCacheHandler) RegisterRoutes(router *mux.Router) {
	router.Path(LogCacheInfoPath).Methods("GET").HandlerFunc(h.unauthenticatedHandlerWrapper.Wrap(h.logCacheInfoHandler))
	router.Path(LogCacheReadPath).Methods("GET").HandlerFunc(h.authenticatedHandlerWrapper.Wrap(h.logCacheReadHandler))
}
