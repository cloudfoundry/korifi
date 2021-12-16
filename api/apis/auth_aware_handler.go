package apis

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
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
