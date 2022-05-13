package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	DropletPath = "/v3/droplets/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFDropletRepository . CFDropletRepository
type CFDropletRepository interface {
	GetDroplet(context.Context, authorization.Info, string) (repositories.DropletRecord, error)
	ListDroplets(context.Context, authorization.Info, repositories.ListDropletsMessage) ([]repositories.DropletRecord, error)
}

type DropletHandler struct {
	serverURL      url.URL
	dropletRepo    CFDropletRepository
	handlerWrapper *AuthAwareHandlerFuncWrapper
}

func NewDropletHandler(
	serverURL url.URL,
	dropletRepo CFDropletRepository,
) *DropletHandler {
	return &DropletHandler{
		handlerWrapper: NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("DropletHandler")),
		serverURL:      serverURL,
		dropletRepo:    dropletRepo,
	}
}

func (h *DropletHandler) dropletGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	dropletGUID := vars["guid"]

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to fetch %s from Kubernetes", repositories.DropletResourceType), "guid", dropletGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *DropletHandler) RegisterRoutes(router *mux.Router) {
	router.Path(DropletPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.dropletGetHandler))
}
