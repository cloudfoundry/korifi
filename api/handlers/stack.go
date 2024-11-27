package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	StacksPath = "/v3/stacks"
)

type StackRepository interface {
	ListStacks(ctx context.Context, authInfo authorization.Info) ([]repositories.StackRecord, error)
}

type Stack struct {
	serverURL url.URL
	stackRepo StackRepository
}

func NewStack(
	serverURL url.URL,
	stackRepo StackRepository,
) *Stack {
	return &Stack{
		serverURL: serverURL,
		stackRepo: stackRepo,
	}
}

func (h *Stack) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.stack.list")

	stacks, err := h.stackRepo.ListStacks(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch stacks from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForStack, stacks, h.serverURL, *r.URL)), nil
}

func (h *Stack) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Stack) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: StacksPath, Handler: h.list},
	}
}
