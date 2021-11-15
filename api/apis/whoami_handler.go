package apis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const (
	WhoAmIEndpoint = "/whoami"
)

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(context.Context, string) (authorization.Identity, error)
}

type WhoAmIHandler struct {
	identityProvider IdentityProvider
	logger           logr.Logger
	apiBaseURL       url.URL
}

func NewWhoAmI(identityProvider IdentityProvider, apiBaseURL url.URL) *WhoAmIHandler {
	return &WhoAmIHandler{
		identityProvider: identityProvider,
		apiBaseURL:       apiBaseURL,
		logger:           controllerruntime.Log.WithName("Org Handler"),
	}
}

func (h *WhoAmIHandler) whoAmIHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	identity, err := h.identityProvider.GetIdentity(ctx, r.Header.Get(headers.Authorization))
	if err != nil {
		writeUnknownErrorResponse(w)
		return
	}

	w.WriteHeader(http.StatusOK)
	identityResponse := presenter.ForWhoAmI(identity)

	err = json.NewEncoder(w).Encode(identityResponse)
	if err != nil {
		h.logger.Error(err, "Failed to write response")
	}
}

func (h *WhoAmIHandler) RegisterRoutes(router *mux.Router) {
	router.Path(WhoAmIEndpoint).Methods("GET").HandlerFunc(h.whoAmIHandler)
}
