package apis

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	WhoAmIPath = "/whoami"
)

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(context.Context, authorization.Info) (authorization.Identity, error)
}

type WhoAmIHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	identityProvider IdentityProvider
	apiBaseURL       url.URL
}

func NewWhoAmI(identityProvider IdentityProvider, apiBaseURL url.URL) *WhoAmIHandler {
	return &WhoAmIHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("WhoAmIHandler")),
		identityProvider: identityProvider,
		apiBaseURL:       apiBaseURL,
	}
}

func (h *WhoAmIHandler) whoAmIHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	identity, err := h.identityProvider.GetIdentity(r.Context(), authInfo)
	if err != nil {
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForWhoAmI(identity)), nil
}

func (h *WhoAmIHandler) RegisterRoutes(router *mux.Router) {
	router.Path(WhoAmIPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.whoAmIHandler))
}
