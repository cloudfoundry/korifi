package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	headers    map[string][]string
}

func NewHandlerResponse(httpStatus int) *HandlerResponse {
	return &HandlerResponse{
		httpStatus: httpStatus,
		headers:    map[string][]string{},
	}
}

func (r *HandlerResponse) WithHeader(key, value string) *HandlerResponse {
	r.headers[key] = append(r.headers[key], value)
	return r
}

func (r *HandlerResponse) WithBody(body interface{}) *HandlerResponse {
	r.body = body
	return r
}

//counterfeiter:generate -o fake -fake-name HandlerFunc . HandlerFunc
//counterfeiter:generate -o fake -fake-name AuthHandlerFunc . AuthHandlerFunc

type (
	HandlerFunc     func(ctx context.Context, logger logr.Logger, r *http.Request) (*HandlerResponse, error)
	AuthHandlerFunc func(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error)
)

type HandlerFuncWrapper struct {
	logger   logr.Logger
	delegate HandlerFunc
}

func NewUnauthenticatedWrapper(logger logr.Logger, delegate HandlerFunc) *HandlerFuncWrapper {
	return &HandlerFuncWrapper{
		logger:   logger,
		delegate: delegate,
	}
}

func NewAuthenticatedWrapper(logger logr.Logger, delegate AuthHandlerFunc) *HandlerFuncWrapper {
	return &HandlerFuncWrapper{
		logger: logger,
		delegate: func(ctx context.Context, logger logr.Logger, r *http.Request) (*HandlerResponse, error) {
			authInfo, ok := authorization.InfoFromContext(r.Context())
			if !ok {
				return nil, errors.New("unable to get auth info")
			}
			return delegate(ctx, logger, authInfo, r)
		},
	}
}

func (h *HandlerFuncWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logger := correlation.AddCorrelationIDToLogger(ctx, h.logger)
	handlerResponse, err := h.delegate(ctx, logger, r)
	if err != nil {
		logger.Info("handler returned error", "error", err)
		presentError(h.logger, w, err)
		return
	}

	if err := handlerResponse.writeTo(w); err != nil {
		_ = apierrors.LogAndReturn(logger, err, "failed to write result to the HTTP response", "handlerResponse", handlerResponse, "method", r.Method, "URL", r.URL)
	}
}

func presentError(logger logr.Logger, w http.ResponseWriter, err error) {
	var apiError apierrors.ApiError
	if errors.As(err, &apiError) {
		writeErr := NewHandlerResponse(apiError.HttpStatus()).
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

		if writeErr != nil {
			_ = apierrors.LogAndReturn(logger, writeErr, "failed to write error to the HTTP response")
		}

		return
	}

	presentError(logger, w, apierrors.NewUnknownError(err))
}

func (response *HandlerResponse) writeTo(w http.ResponseWriter) error {
	for header, headerValues := range response.headers {
		for _, value := range headerValues {
			w.Header().Add(header, value)
		}
	}

	if response.body == nil {
		w.WriteHeader(response.httpStatus)
		return nil
	}

	w.Header().Set(headers.ContentType, "application/json")
	w.WriteHeader(response.httpStatus)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(response.body)
	if err != nil {
		return fmt.Errorf("failed to encode and write response: %w", err)
	}

	return nil
}
