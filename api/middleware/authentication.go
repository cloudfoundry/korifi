package middleware

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(context.Context, authorization.Info) (authorization.Identity, error)
}

//counterfeiter:generate -o fake -fake-name AuthInfoParser . AuthInfoParser

type authentication struct {
	authInfoParser   AuthInfoParser
	identityProvider IdentityProvider
}

type AuthInfoParser interface {
	Parse(authHeader string) (authorization.Info, error)
}

func Authentication(
	authInfoParser AuthInfoParser,
	identityProvider IdentityProvider,
) func(http.Handler) http.Handler {
	return (&authentication{
		authInfoParser:   authInfoParser,
		identityProvider: identityProvider,
	}).middleware
}

func (a *authentication) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logr.FromContextOrDiscard(r.Context()).WithName("authentication-middleware")

		authInfo, err := a.authInfoParser.Parse(r.Header.Get(headers.Authorization))
		if err != nil {
			logger.Info("failed to parse auth info", "reason", err)
			routing.PresentError(logger, w, err)
			return
		}

		r = r.WithContext(authorization.NewContext(r.Context(), &authInfo))

		_, err = a.identityProvider.GetIdentity(r.Context(), authInfo)
		if err != nil {
			routing.PresentError(logger, w, apierrors.LogAndReturn(logger, err, "failed to get identity"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
