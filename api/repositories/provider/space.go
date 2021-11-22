package provider

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/go-http-utils/headers"
)

type SpaceRepositoryProvider struct {
	spaceRepo        repositories.CFSpaceRepository
	authNsProvider   repositories.AuthorizedNamespacesProvider
	identityProvider IdentityProvider
}

func NewSpace(
	spaceRepo repositories.CFSpaceRepository,
	authNsProvider repositories.AuthorizedNamespacesProvider,
	identityProvider IdentityProvider) *SpaceRepositoryProvider {
	return &SpaceRepositoryProvider{
		spaceRepo:        spaceRepo,
		authNsProvider:   authNsProvider,
		identityProvider: identityProvider,
	}
}

func (p *SpaceRepositoryProvider) SpaceRepoForRequest(request *http.Request) (repositories.CFSpaceRepository, error) {
	identity, err := p.identityProvider.GetIdentity(request.Context(), request.Header.Get(headers.Authorization))
	if err != nil {
		return nil, err
	}

	return repositories.NewSpaceRepoAuthDecorator(p.spaceRepo, identity, p.authNsProvider), nil
}

type PrivilegedSpaceRepositoryProvider struct {
	spaceRepo repositories.CFSpaceRepository
}

func NewPrivilegedSpace(spaceRepo repositories.CFSpaceRepository) *PrivilegedSpaceRepositoryProvider {
	return &PrivilegedSpaceRepositoryProvider{
		spaceRepo: spaceRepo,
	}
}

func (p *PrivilegedSpaceRepositoryProvider) SpaceRepoForRequest(_ *http.Request) (repositories.CFSpaceRepository, error) {
	return p.spaceRepo, nil
}
