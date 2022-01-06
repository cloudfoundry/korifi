package repositories

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
)

type SpaceRepoAuthDecorator struct {
	CFSpaceRepository
	authInfo   authorization.Info
	nsProvider AuthorizedNamespacesProvider
}

func NewSpaceRepoAuthDecorator(
	repo CFSpaceRepository,
	authInfo authorization.Info,
	nsProvider AuthorizedNamespacesProvider,
) *SpaceRepoAuthDecorator {
	return &SpaceRepoAuthDecorator{
		CFSpaceRepository: repo,
		authInfo:          authInfo,
		nsProvider:        nsProvider,
	}
}

func (r *SpaceRepoAuthDecorator) ListSpaces(ctx context.Context, orgUIDs []string, spaceNames []string) ([]SpaceRecord, error) {
	spaces, err := r.CFSpaceRepository.ListSpaces(ctx, orgUIDs, spaceNames)
	if err != nil {
		return nil, err
	}

	authorizedNamespaces, err := r.nsProvider.GetAuthorizedSpaceNamespaces(ctx, r.authInfo)
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
