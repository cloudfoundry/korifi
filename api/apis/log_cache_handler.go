package apis

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/repositories"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"

	"github.com/go-logr/logr"
	"github.com/go-playground/validator"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
)

const (
	LogCacheInfoPath = "/api/v1/info"
	LogCacheReadPath = "/api/v1/read/{guid}"
	logCacheVersion  = "2.11.4+cf-k8s"
)

//counterfeiter:generate -o fake -fake-name ReadAppLogs . ReadAppLogsAction
type ReadAppLogsAction func(ctx context.Context, authInfo authorization.Info, appGUID string, read payloads.LogRead) ([]repositories.LogRecord, error)

// LogCacheHandler implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workflow.
type LogCacheHandler struct {
	logger            logr.Logger
	appRepo           CFAppRepository
	buildRepo         CFBuildRepository
	readAppLogsAction ReadAppLogsAction
}

func NewLogCacheHandler(logger logr.Logger, appRepo CFAppRepository,
	buildRepository CFBuildRepository, readAppLogsAction ReadAppLogsAction,
) *LogCacheHandler {
	return &LogCacheHandler{
		logger:            logger,
		appRepo:           appRepo,
		buildRepo:         buildRepository,
		readAppLogsAction: readAppLogsAction,
	}
}

func (h *LogCacheHandler) logCacheInfoHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
		"version":   logCacheVersion,
		"vm_uptime": "0",
	}), nil
}

func (h *LogCacheHandler) logCacheReadHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	logReadPayload := new(payloads.LogRead)
	err := schema.NewDecoder().Decode(logReadPayload, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in log read payload")
					return nil, apierrors.NewUnknownKeyError(err, logReadPayload.SupportedFilterKeys())
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	v := validator.New()
	if logReadPayloadErr := v.Struct(logReadPayload); logReadPayloadErr != nil {
		h.logger.Error(logReadPayloadErr, "Error validating log read request query parameters")
		return nil, apierrors.NewUnprocessableEntityError(logReadPayloadErr, "error validating log read query parameters")
	}

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var logs []repositories.LogRecord
	logs, err = h.readAppLogsAction(ctx, authInfo, appGUID, *logReadPayload)
	if err != nil {
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForLogs(logs)), nil
}

func (h *LogCacheHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(LogCacheInfoPath).Methods("GET").HandlerFunc(w.Wrap(h.logCacheInfoHandler))
	router.Path(LogCacheReadPath).Methods("GET").HandlerFunc(w.Wrap(h.logCacheReadHandler))
}
