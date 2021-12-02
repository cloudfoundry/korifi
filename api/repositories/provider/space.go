package provider

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
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
	authInfo, ok := authorization.InfoFromContext(request.Context())
	if !ok {
		return nil, errors.New("no authorization info in the request context")
	}

	identity, err := p.identityProvider.GetIdentity(request.Context(), authInfo)
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
