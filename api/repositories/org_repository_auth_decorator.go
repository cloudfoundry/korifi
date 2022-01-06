package repositories

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
)

//counterfeiter:generate -o fake -fake-name AuthorizedNamespacesProvider . AuthorizedNamespacesProvider

type AuthorizedNamespacesProvider interface {
	GetAuthorizedOrgNamespaces(context.Context, authorization.Info) ([]string, error)
	GetAuthorizedSpaceNamespaces(context.Context, authorization.Info) ([]string, error)
}

type OrgRepoAuthDecorator struct {
	CFOrgRepository
	nsProvider AuthorizedNamespacesProvider
}

func NewOrgRepoAuthDecorator(repo CFOrgRepository, nsProvider AuthorizedNamespacesProvider) *OrgRepoAuthDecorator {
	return &OrgRepoAuthDecorator{
		CFOrgRepository: repo,
		nsProvider:      nsProvider,
	}
}

func (r *OrgRepoAuthDecorator) ListOrgs(ctx context.Context, info authorization.Info, names []string) ([]OrgRecord, error) {
	orgs, err := r.CFOrgRepository.ListOrgs(ctx, info, names)
	if err != nil {
		return nil, err
	}

	authorizedNamespaces, err := r.nsProvider.GetAuthorizedOrgNamespaces(ctx, info)
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
