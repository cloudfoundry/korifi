package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/correlation"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

type AuthenticationMiddleware struct {
	logger                   logr.Logger
	authInfoParser           AuthInfoParser
	identityProvider         IdentityProvider
	unauthenticatedEndpoints map[string]interface{}
}

//counterfeiter:generate -o fake -fake-name AuthInfoParser . AuthInfoParser

type AuthInfoParser interface {
	Parse(authHeader string) (authorization.Info, error)
}

func NewAuthenticationMiddleware(authInfoParser AuthInfoParser, identityProvider IdentityProvider) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		logger:           ctrl.Log.WithName("AuthenticationMiddleware"),
		authInfoParser:   authInfoParser,
		identityProvider: identityProvider,
		unauthenticatedEndpoints: map[string]interface{}{
			"/":            struct{}{},
			"/v3":          struct{}{},
			"/api/v1/info": struct{}{},
		},
	}
}

func (a *AuthenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.isUnauthenticatedEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		logger := correlation.AddCorrelationIDToLogger(ctx, a.logger)

		authInfo, err := a.authInfoParser.Parse(r.Header.Get(headers.Authorization))
		if err != nil {
			logger.Error(err, "failed to parse auth info")
			presentError(w, err)
			return
		}

		r = r.WithContext(authorization.NewContext(r.Context(), &authInfo))

		_, err = a.identityProvider.GetIdentity(r.Context(), authInfo)
		if err != nil {
			logger.Error(err, "failed to get identity")
			presentError(w, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *AuthenticationMiddleware) isUnauthenticatedEndpoint(p string) bool {
	_, authNotRequired := a.unauthenticatedEndpoints[p]

	return authNotRequired
}
