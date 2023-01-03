package handlers

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt"
)

const (
	OAuthTokenPath = "/oauth/token"
)

type OAuthTokenHandler struct {
	apiBaseURL url.URL
}

func NewOAuthToken(apiBaseURL url.URL) *OAuthTokenHandler {
	return &OAuthTokenHandler{
		apiBaseURL: apiBaseURL,
	}
}

func (h *OAuthTokenHandler) oauthTokenHandler(ctx context.Context, logger logr.Logger, r *http.Request) (*HandlerResponse, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte("not-a-real-secret"))
	if err != nil {
		// we don't expect to hit this ever, given that the string above is hard coded.
		panic(err.Error())
	}

	return NewHandlerResponse(http.StatusOK).WithBody(map[string]string{
		"token_type":   "bearer",
		"access_token": tokenString,
	}), nil
}

func (h *OAuthTokenHandler) UnauthenticatedRoutes() []Route {
	return []Route{
		{Method: "POST", Pattern: OAuthTokenPath, HandlerFunc: h.oauthTokenHandler},
	}
}

func (h *OAuthTokenHandler) AuthenticatedRoutes() []AuthRoute {
	return []AuthRoute{}
}
