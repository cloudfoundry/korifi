package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	usersPath = "/v3/users"
)

type User struct {
	apiBaseURL url.URL
}

func NewUser(apiBaseURL url.URL) User {
	return User{
		apiBaseURL: apiBaseURL,
	}
}

func (h User) list(req *http.Request) (*routing.Response, error) {
	usernames := req.URL.Query().Get("usernames")
	users := []interface{}{}
	if len(usernames) > 0 {
		for _, username := range strings.Split(usernames, ",") {
			users = append(users, presenter.ForUser(username))
		}
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(users, h.apiBaseURL, *req.URL)), nil
}

func (h User) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h User) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: usersPath, Handler: h.list},
	}
}
