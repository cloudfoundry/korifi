package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cacheTTL = 120 * time.Second
)

type CFUserMiddleware struct {
	privilegedClient                client.Client
	identityProvider                IdentityProvider
	cfRootNamespace                 string
	cfUserCache                     *cache.Expiring
	unauthenticatedEndpointRegistry UnauthenticatedEndpointRegistry
}

func NewCFUserMiddleware(
	privilegedClient client.Client,
	identityProvider IdentityProvider,
	cfRootNamespace string,
	cfUserCache *cache.Expiring,
	unauthenticatedEndpointRegistry UnauthenticatedEndpointRegistry,
) *CFUserMiddleware {
	return &CFUserMiddleware{
		privilegedClient:                privilegedClient,
		identityProvider:                identityProvider,
		cfRootNamespace:                 cfRootNamespace,
		cfUserCache:                     cfUserCache,
		unauthenticatedEndpointRegistry: unauthenticatedEndpointRegistry,
	}
}

func (m *CFUserMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.unauthenticatedEndpointRegistry.IsUnauthenticatedEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

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

func (m *CFUserMiddleware) isCFUser(ctx context.Context, identity authorization.Identity) (bool, error) {
	_, isCFUser := m.cfUserCache.Get(identity.Hash())
	if isCFUser {
		return true, nil
	}

	roleBindings := &rbacv1.RoleBindingList{}
	err := m.privilegedClient.List(ctx, roleBindings, client.InNamespace(m.cfRootNamespace))
	if err != nil {
		return false, err
	}

	for _, rb := range roleBindings.Items {
		for _, subj := range rb.Subjects {
			if subj.Name == identity.Name && subj.Kind == identity.Kind {
				m.cfUserCache.Set(identity.Hash(), struct{}{}, cacheTTL)
				return true, nil
			}
		}
	}

	return false, nil
}
