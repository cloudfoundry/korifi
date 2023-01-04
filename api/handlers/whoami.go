package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
)

const (
	WhoAmIPath = "/whoami"
)

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(context.Context, authorization.Info) (authorization.Identity, error)
}

type WhoAmI struct {
	identityProvider IdentityProvider
	apiBaseURL       url.URL
}

func NewWhoAmI(identityProvider IdentityProvider, apiBaseURL url.URL) *WhoAmI {
	return &WhoAmI{
		identityProvider: identityProvider,
		apiBaseURL:       apiBaseURL,
	}
}

func (h *WhoAmI) whoAmI(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.whoami")

	identity, err := h.identityProvider.GetIdentity(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get identity")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForWhoAmI(identity)), nil
}

func (h *WhoAmI) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", WhoAmIPath, routing.Handler(h.whoAmI))
}
