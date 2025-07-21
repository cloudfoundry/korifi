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
	usersPath = "/v3/users"
)

//counterfeiter:generate -o fake -fake-name UserRepository . UserRepository

type UserRepository interface {
	ListUsers(ctx context.Context, authInfo authorization.Info, message repositories.ListUsersMessage) (repositories.ListResult[repositories.UserRecord], error)
}

type User struct {
	serverURL        url.URL
	requestValidator RequestValidator
	userRepo         UserRepository
}

func NewUser(
	apiBaseURL url.URL,
	userRepo UserRepository,
	requestValidator RequestValidator,
) User {
	return User{
		serverURL:        apiBaseURL,
		userRepo:         userRepo,
		requestValidator: requestValidator,
	}
}

func (h User) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.user.list")

	payload := new(payloads.UserList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode and validate payload")
	}

	users, err := h.userRepo.ListUsers(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to fetch users")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForUser, users, h.serverURL, *r.URL)), nil
}

func (h User) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h User) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: usersPath, Handler: h.list},
	}
}
