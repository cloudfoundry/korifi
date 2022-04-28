package apis

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"

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

func NewAuthenticationMiddleware(logger logr.Logger, authInfoParser AuthInfoParser, identityProvider IdentityProvider) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		logger:           logger,
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

		authInfo, err := a.authInfoParser.Parse(r.Header.Get(headers.Authorization))
		if err != nil {
			a.logger.Error(err, "failed to parse auth info")
			presentError(w, err)
			return
		}

		r = r.WithContext(authorization.NewContext(r.Context(), &authInfo))

		_, err = a.identityProvider.GetIdentity(r.Context(), authInfo)
		if err != nil {
			a.logger.Error(err, "failed to get identity")
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
