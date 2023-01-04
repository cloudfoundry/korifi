package handlers

import (
	"net/http"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-chi/chi"
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

func (h *OAuthTokenHandler) oauthTokenHandler(r *http.Request) (*routing.Response, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte("not-a-real-secret"))
	if err != nil {
		// we don't expect to hit this ever, given that the string above is hard coded.
		panic(err.Error())
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(map[string]string{
		"token_type":   "bearer",
		"access_token": tokenString,
	}), nil
}

func (h *OAuthTokenHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("POST", OAuthTokenPath, routing.Handler(h.oauthTokenHandler))
}
