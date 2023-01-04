package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/presenter"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

type Response struct {
	httpStatus int
	body       interface{}
	headers    map[string][]string
}

func NewResponse(httpStatus int) *Response {
	return &Response{
		httpStatus: httpStatus,
		headers:    map[string][]string{},
	}
}

func (r *Response) WithHeader(key, value string) *Response {
	r.headers[key] = append(r.headers[key], value)
	return r
}

func (r *Response) WithBody(body interface{}) *Response {
	r.body = body
	return r
}

//counterfeiter:generate -o fake -fake-name Handler . Handler

type Handler func(r *http.Request) (*Response, error)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := logr.FromContextOrDiscard(r.Context())

	handlerResponse, err := h(r)
	if err != nil {
		logger.Info("handler returned error", "error", err)
		PresentError(logger, w, err)
		return
	}

	if err := handlerResponse.writeTo(w); err != nil {
		_ = apierrors.LogAndReturn(logger, err, "failed to write result to the HTTP response", "handlerResponse", handlerResponse, "method", r.Method, "URL", r.URL)
	}
}

func PresentError(logger logr.Logger, w http.ResponseWriter, err error) {
	var apiError apierrors.ApiError
	if errors.As(err, &apiError) {
		writeErr := NewResponse(apiError.HttpStatus()).
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

	PresentError(logger, w, apierrors.NewUnknownError(err))
}

func (response *Response) writeTo(w http.ResponseWriter) error {
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
