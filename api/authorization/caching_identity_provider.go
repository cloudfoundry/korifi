package authorization

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/cache"
)

const (
	cacheTTL = 120 * time.Second
)

type CachingIdentityProvider struct {
	identityProvider IdentityProvider
	identityCache    *cache.Expiring
}

func NewCachingIdentityProvider(identityProvider IdentityProvider, identityCache *cache.Expiring) *CachingIdentityProvider {
	return &CachingIdentityProvider{
		identityProvider: identityProvider,
		identityCache:    identityCache,
	}
}

func (p *CachingIdentityProvider) GetIdentity(ctx context.Context, info Info) (Identity, error) {
	idInterface, ok := p.identityCache.Get(info.Hash())
	if ok {
		id, castOK := idInterface.(Identity)
		if castOK {
			return id, nil
		}
		return Identity{}, fmt.Errorf("identity-provider cache: expected authorization.Identity{}, got %T", idInterface)
	}

	identity, err := p.identityProvider.GetIdentity(ctx, info)
	if err == nil {
		p.identityCache.Set(info.Hash(), identity, cacheTTL)
	}

	return identity, err
}
