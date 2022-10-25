package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/correlation"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

//counterfeiter:generate -o fake -fake-name UnauthenticatedEndpointRegistry . UnauthenticatedEndpointRegistry
//counterfeiter:generate -o fake -fake-name AuthInfoParser . AuthInfoParser

type UnauthenticatedEndpointRegistry interface {
	IsUnauthenticatedEndpoint(requestPath string) bool
}

type AuthenticationMiddleware struct {
	logger                          logr.Logger
	authInfoParser                  AuthInfoParser
	identityProvider                IdentityProvider
	unauthenticatedEndpointRegistry UnauthenticatedEndpointRegistry
}

type AuthInfoParser interface {
	Parse(authHeader string) (authorization.Info, error)
}

func NewAuthenticationMiddleware(
	authInfoParser AuthInfoParser,
	identityProvider IdentityProvider,
	unauthenticatedEndpointRegistry UnauthenticatedEndpointRegistry,
) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		logger:                          ctrl.Log.WithName("AuthenticationMiddleware"),
		authInfoParser:                  authInfoParser,
		identityProvider:                identityProvider,
		unauthenticatedEndpointRegistry: unauthenticatedEndpointRegistry,
	}
}

func (a *AuthenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.unauthenticatedEndpointRegistry.IsUnauthenticatedEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		logger := correlation.AddCorrelationIDToLogger(ctx, a.logger)

		authInfo, err := a.authInfoParser.Parse(r.Header.Get(headers.Authorization))
		if err != nil {
			logger.Info("failed to parse auth info", "err", err)
			presentError(logger, w, err)
			return
		}

		r = r.WithContext(authorization.NewContext(r.Context(), &authInfo))

		_, err = a.identityProvider.GetIdentity(r.Context(), authInfo)
		if err != nil {
			presentError(logger, w, apierrors.LogAndReturn(logger, err, "failed to get identity"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
