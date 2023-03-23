package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"k8s.io/apimachinery/pkg/util/cache"
)

const (
	cacheTTL = 120 * time.Second
)

type cfUser struct {
	nsPermissionChecker NamespacePermissionChecker
	identityProvider    IdentityProvider
	cfRootNamespace     string
	cfUserCache         *cache.Expiring
}

//counterfeiter:generate -o fake -fake-name NamespacePermissionChecker . NamespacePermissionChecker

type NamespacePermissionChecker interface {
	AuthorizedIn(context.Context, authorization.Identity, string) (bool, error)
}

func CFUser(
	nsPermissionChecker NamespacePermissionChecker,
	identityProvider IdentityProvider,
	cfRootNamespace string,
	cfUserCache *cache.Expiring,
) func(http.Handler) http.Handler {
	return (&cfUser{
		nsPermissionChecker: nsPermissionChecker,
		identityProvider:    identityProvider,
		cfRootNamespace:     cfRootNamespace,
		cfUserCache:         cfUserCache,
	}).middleware
}

func (m *cfUser) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authInfo, ok := authorization.InfoFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		identity, err := m.identityProvider.GetIdentity(r.Context(), authInfo)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		isCFUser, err := m.isCFUser(r.Context(), identity)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if !isCFUser {
			w.Header().Add("X-Cf-Warnings", fmt.Sprintf("Warning: subject '%s/%s' has no CF roles assigned. This is not supported and may cause unexpected behaviour.", identity.Kind, identity.Name))
		}

		next.ServeHTTP(w, r)
	})
}

func (m *cfUser) isCFUser(ctx context.Context, identity authorization.Identity) (bool, error) {
	_, isCFUser := m.cfUserCache.Get(identity.Hash())
	if isCFUser {
		return true, nil
	}

	authorized, err := m.nsPermissionChecker.AuthorizedIn(ctx, identity, m.cfRootNamespace)
	if err != nil {
		return false, err
	}
	if authorized {
		m.cfUserCache.Set(identity.Hash(), struct{}{}, cacheTTL)
		return true, nil
	}
	return false, nil
}
