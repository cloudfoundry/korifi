package provider

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type SpaceRepositoryProvider struct {
	spaceRepo      repositories.CFSpaceRepository
	authNsProvider repositories.AuthorizedNamespacesProvider
}

func NewSpace(
	spaceRepo repositories.CFSpaceRepository,
	authNsProvider repositories.AuthorizedNamespacesProvider,
) *SpaceRepositoryProvider {
	return &SpaceRepositoryProvider{
		spaceRepo:      spaceRepo,
		authNsProvider: authNsProvider,
	}
}

func (p *SpaceRepositoryProvider) SpaceRepoForRequest() (repositories.CFSpaceRepository, error) {
	return repositories.NewSpaceRepoAuthDecorator(p.spaceRepo, p.authNsProvider), nil
}

type PrivilegedSpaceRepositoryProvider struct {
	spaceRepo repositories.CFSpaceRepository
}

func NewPrivilegedSpace(spaceRepo repositories.CFSpaceRepository) *PrivilegedSpaceRepositoryProvider {
	return &PrivilegedSpaceRepositoryProvider{
		spaceRepo: spaceRepo,
	}
}

func (p *PrivilegedSpaceRepositoryProvider) SpaceRepoForRequest() (repositories.CFSpaceRepository, error) {
	return p.spaceRepo, nil
}
