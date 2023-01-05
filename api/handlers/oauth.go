package handlers

import (
	"net/http"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/golang-jwt/jwt"
)

const (
	OAuthTokenPath = "/oauth/token"
)

type OAuth struct {
	apiBaseURL url.URL
}

func NewOAuth(apiBaseURL url.URL) *OAuth {
	return &OAuth{
		apiBaseURL: apiBaseURL,
	}
}

func (h *OAuth) token(r *http.Request) (*routing.Response, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte("not-a-real-secret"))
	if err != nil {
		// we don't expect to hit this ever, given that the string above is hard coded.
		panic(err.Error())
	}

	return routing.NewResponse(http.StatusOK).WithBody(map[string]string{
		"token_type":   "bearer",
		"access_token": tokenString,
	}), nil
}

func (h *OAuth) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: OAuthTokenPath, Handler: h.token},
	}
}

func (h *OAuth) AuthenticatedRoutes() []routing.Route {
	return nil
}
