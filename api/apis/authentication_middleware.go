package apis

import (
	"net/http"
	"regexp"

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

// Eventually we will want to authenticate the log-cache read endpoint and go back to using the simple
// unauthenticatedEndpoints map. For now though we need to do a more complicated regexp match against
// the path since the log-cache read endpoint includes an arbitrary guid as part of its path
var logCacheReadEndpointRegexp = regexp.MustCompile(`\/api\/v1\/read\/[0-9a-fA-F\-]*$`)

func (a *AuthenticationMiddleware) isUnauthenticatedEndpoint(p string) bool {
	_, authNotRequired := a.unauthenticatedEndpoints[p]

	return authNotRequired || logCacheReadEndpointRegexp.MatchString(p)
}
