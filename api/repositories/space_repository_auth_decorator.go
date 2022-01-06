package repositories

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
)

type SpaceRepoAuthDecorator struct {
	CFSpaceRepository
	nsProvider AuthorizedNamespacesProvider
}

func NewSpaceRepoAuthDecorator(
	repo CFSpaceRepository,
	nsProvider AuthorizedNamespacesProvider,
) *SpaceRepoAuthDecorator {
	return &SpaceRepoAuthDecorator{
		CFSpaceRepository: repo,
		nsProvider:        nsProvider,
	}
}

func (r *SpaceRepoAuthDecorator) ListSpaces(ctx context.Context, info authorization.Info, orgUIDs []string, spaceNames []string) ([]SpaceRecord, error) {
	spaces, err := r.CFSpaceRepository.ListSpaces(ctx, info, orgUIDs, spaceNames)
	if err != nil {
		return nil, err
	}

	authorizedNamespaces, err := r.nsProvider.GetAuthorizedSpaceNamespaces(ctx, info)
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
