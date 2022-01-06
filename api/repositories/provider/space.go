package provider

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
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

func (p *SpaceRepositoryProvider) SpaceRepoForRequest(request *http.Request) (repositories.CFSpaceRepository, error) {
	authInfo, ok := authorization.InfoFromContext(request.Context())
	if !ok {
		return nil, errors.New("no authorization info in the request context")
	}

	return repositories.NewSpaceRepoAuthDecorator(p.spaceRepo, authInfo, p.authNsProvider), nil
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
