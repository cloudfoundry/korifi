package repositories

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
)

type SpaceRepoAuthDecorator struct {
	CFSpaceRepository
	identity   authorization.Identity
	nsProvider AuthorizedNamespacesProvider
}

func NewSpaceRepoAuthDecorator(
	repo CFSpaceRepository,
	identity authorization.Identity,
	nsProvider AuthorizedNamespacesProvider,
) *SpaceRepoAuthDecorator {
	return &SpaceRepoAuthDecorator{
		CFSpaceRepository: repo,
		identity:          identity,
		nsProvider:        nsProvider,
	}
}

func (r *SpaceRepoAuthDecorator) FetchSpaces(ctx context.Context, orgUIDs []string, spaceNames []string) ([]SpaceRecord, error) {
	spaces, err := r.CFSpaceRepository.FetchSpaces(ctx, orgUIDs, spaceNames)
	if err != nil {
		return nil, err
	}

	authorizedNamespaces, err := r.nsProvider.GetAuthorizedNamespaces(ctx, r.identity)
	if err != nil {
		return nil, err
	}

	spacesFilter := toMap(authorizedNamespaces)

	result := []SpaceRecord{}
	for _, space := range spaces {
		if _, ok := spacesFilter[space.GUID]; !ok {
			continue
		}

		result = append(result, space)
	}

	return result, nil
}
