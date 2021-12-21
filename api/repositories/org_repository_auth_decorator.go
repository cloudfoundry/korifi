package repositories

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
)

//counterfeiter:generate -o fake -fake-name AuthorizedNamespacesProvider . AuthorizedNamespacesProvider

type AuthorizedNamespacesProvider interface {
	GetAuthorizedNamespaces(context.Context, authorization.Identity) ([]string, error)
}

type OrgRepoAuthDecorator struct {
	CFOrgRepository
	identity   authorization.Identity
	nsProvider AuthorizedNamespacesProvider
}

func NewOrgRepoAuthDecorator(
	repo CFOrgRepository,
	identity authorization.Identity,
	nsProvider AuthorizedNamespacesProvider,
) *OrgRepoAuthDecorator {
	return &OrgRepoAuthDecorator{
		CFOrgRepository: repo,
		identity:        identity,
		nsProvider:      nsProvider,
	}
}

func (r *OrgRepoAuthDecorator) ListOrgs(ctx context.Context, names []string) ([]OrgRecord, error) {
	orgs, err := r.CFOrgRepository.ListOrgs(ctx, names)
	if err != nil {
		return nil, err
	}

	authorizedNamespaces, err := r.nsProvider.GetAuthorizedNamespaces(ctx, r.identity)
	if err != nil {
		return nil, err
	}

	orgsFilter := toMap(authorizedNamespaces)

	result := []OrgRecord{}
	for _, org := range orgs {
		if _, ok := orgsFilter[org.GUID]; !ok {
			continue
		}

		result = append(result, org)
	}

	return result, nil
}
