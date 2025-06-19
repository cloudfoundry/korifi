package descriptors

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

//counterfeiter:generate -o fake -fake-name DiscoveryInterface k8s.io/client-go/discovery.DiscoveryInterface
type CachingPluralizer struct {
	discoveryClient discovery.DiscoveryInterface
	cache           *sync.Map
}

func NewCachingPluralizer(dc discovery.DiscoveryInterface) *CachingPluralizer {
	return &CachingPluralizer{
		discoveryClient: dc,
		cache:           new(sync.Map),
	}
}

func (p *CachingPluralizer) Pluralize(resourceGVK schema.GroupVersionKind) (string, error) {
	if plural, found := p.cache.Load(resourceGVK.String()); found {
		return plural.(string), nil
	}

	gv := resourceGVK.GroupVersion().String()

	resources, err := p.discoveryClient.ServerResourcesForGroupVersion(gv)
	if err != nil {
		return "", fmt.Errorf("failed to fetch resources for %s: %w", gv, err)
	}

	for _, r := range resources.APIResources {
		if r.Kind == resourceGVK.Kind {
			p.cache.Store(resourceGVK.String(), r.Name)
			return r.Name, nil
		}
	}

	return "", fmt.Errorf("kind %q not found in group/version %q", resourceGVK.Kind, gv)
}
