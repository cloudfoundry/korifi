package apis

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierr"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

//counterfeiter:generate -o fake -fake-name AuthAwareHandlerFunc . AuthAwareHandlerFunc

type AuthAwareHandlerFunc func(authInfo authorization.Info, w http.ResponseWriter, r *http.Request)

type AuthAwareHandlerFuncWrapper struct {
	logger logr.Logger
}

func NewAuthAwareHandlerFuncWrapper(logger logr.Logger) *AuthAwareHandlerFuncWrapper {
	return &AuthAwareHandlerFuncWrapper{logger: logger}
}

func (wrapper *AuthAwareHandlerFuncWrapper) Wrap(delegate AuthAwareHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authInfo, ok := authorization.InfoFromContext(r.Context())
		if !ok {
			wrapper.logger.Error(nil, "unable to get auth info")
			w.Header().Add(headers.ContentType, "application/json")
			writeUnknownErrorResponse(w)
			return
		}

		delegate(authInfo, w, r)
	}
}

type AuthAwareHandlerFunc__NewStyle func(authInfo authorization.Info, r *http.Request) (int, interface{}, error)

type AuthAwareHandlerFuncWrapper__NewStyle struct {
	logger logr.Logger
}

func NewAuthAwareHandlerFuncWrapper__NewStyle(logger logr.Logger) *AuthAwareHandlerFuncWrapper__NewStyle {
	return &AuthAwareHandlerFuncWrapper__NewStyle{logger: logger}
}

func (wrapper *AuthAwareHandlerFuncWrapper__NewStyle) Wrap(delegate AuthAwareHandlerFunc__NewStyle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add(headers.ContentType, "application/json")

		authInfo, ok := authorization.InfoFromContext(r.Context())
		if !ok {
			wrapper.logger.Error(nil, "unable to get auth info")
			writeErrorResponse(w, presenter.ForError(apierr.NewUnknownError(nil)))
			return
		}

		status, result, err := delegate(authInfo, r)
		if err != nil {
			writeErrorResponse(w, presenter.ForError(err))
			return
		}

		writeResponse(w, status, result)
	}
}
