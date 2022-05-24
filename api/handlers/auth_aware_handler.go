package apis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/correlation"
	"code.cloudfoundry.org/korifi/api/presenter"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

type HandlerResponse struct {
	httpStatus int
	body       interface{}
	headers    map[string]string
}

func NewHandlerResponse(httpStatus int) *HandlerResponse {
	return &HandlerResponse{
		httpStatus: httpStatus,
		headers:    map[string]string{},
	}
}

func (r *HandlerResponse) WithHeader(key, value string) *HandlerResponse {
	r.headers[key] = value
	return r
}

func (r *HandlerResponse) WithBody(body interface{}) *HandlerResponse {
	r.body = body
	return r
}

//counterfeiter:generate -o fake -fake-name AuthAwareHandlerFunc . AuthAwareHandlerFunc

type AuthAwareHandlerFunc func(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error)

type AuthAwareHandlerFuncWrapper struct {
	logger logr.Logger
}

func NewAuthAwareHandlerFuncWrapper(logger logr.Logger) *AuthAwareHandlerFuncWrapper {
	return &AuthAwareHandlerFuncWrapper{logger: logger}
}

func (h *AuthAwareHandlerFuncWrapper) Wrap(delegate AuthAwareHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		logger := correlation.AddCorrelationIDToLogger(ctx, h.logger)
		authInfo, ok := authorization.InfoFromContext(r.Context())
		if !ok {
			logger.Error(nil, "unable to get auth info")
			presentError(w, nil)
			return
		}

		handlerResponse, err := delegate(ctx, logger, authInfo, r)
		if err != nil {
			logger.Info("handler returned error", "error", err)
			presentError(w, err)
			return
		}

		handlerResponse.writeTo(w)
	}
}

func presentError(w http.ResponseWriter, err error) {
	var apiError apierrors.ApiError
	if errors.As(err, &apiError) {
		NewHandlerResponse(apiError.HttpStatus()).
			WithBody(presenter.ErrorsResponse{
				Errors: []presenter.PresentedError{
					{
						Detail: apiError.Detail(),
						Title:  apiError.Title(),
						Code:   apiError.Code(),
					},
				},
			}).
			writeTo(w)
		return
	}

	presentError(w, apierrors.NewUnknownError(err))
}

func (response *HandlerResponse) writeTo(w http.ResponseWriter) {
	for k, v := range response.headers {
		w.Header().Set(k, v)
	}

	if response.body == nil {
		w.WriteHeader(response.httpStatus)
		return
	}

	w.Header().Set(headers.ContentType, "application/json")
	w.WriteHeader(response.httpStatus)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(response.body)
	if err != nil {
		Logger.Error(err, "failed to encode and write response")
	}
}
