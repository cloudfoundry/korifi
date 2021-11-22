package apis

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"github.com/go-http-utils/headers"
)

type AuthenticationMiddleware struct {
	identityProvider         IdentityProvider
	unauthenticatedEndpoints map[string]interface{}
}

func NewAuthenticationMiddleware(identityProvider IdentityProvider) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		identityProvider: identityProvider,
		unauthenticatedEndpoints: map[string]interface{}{
			"/":   struct{}{},
			"/v3": struct{}{},
		},
	}
}

func (a *AuthenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, authNotRequired := a.unauthenticatedEndpoints[r.URL.Path]; authNotRequired {
			next.ServeHTTP(w, r)
			return
		}

		_, err := a.identityProvider.GetIdentity(r.Context(), r.Header.Get(headers.Authorization))

		if authorization.IsInvalidAuth(err) {
			writeInvalidAuthErrorResponse(w)
			return
		}
		if authorization.IsNotAuthenticated(err) {
			writeNotAuthenticatedErrorResponse(w)
			return
		}
		if err != nil {
			writeUnknownErrorResponse(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}
