//nolint:dupl
package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	StacksPath = "/v3/stacks"
)

//counterfeiter:generate -o fake -fake-name StackRepository . StackRepository

type StackRepository interface {
	ListStacks(ctx context.Context, authInfo authorization.Info, message repositories.ListStacksMessage) (repositories.ListResult[repositories.StackRecord], error)
}

type Stack struct {
	serverURL        url.URL
	requestValidator RequestValidator
	stackRepo        StackRepository
}

func NewStack(
	serverURL url.URL,
	stackRepo StackRepository,
	requestValidator RequestValidator,
) *Stack {
	return &Stack{
		serverURL:        serverURL,
		stackRepo:        stackRepo,
		requestValidator: requestValidator,
	}
}

func (h *Stack) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.stack.list")

	payload := new(payloads.StackList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to decode and validate StackList payload")
	}

	stacks, err := h.stackRepo.ListStacks(r.Context(), authInfo, payload.ToMessage())
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
