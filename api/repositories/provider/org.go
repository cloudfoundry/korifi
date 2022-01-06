package provider

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type OrgRepositoryProvider struct {
	orgRepo        repositories.CFOrgRepository
	authNsProvider repositories.AuthorizedNamespacesProvider
}

func NewOrg(
	orgRepo repositories.CFOrgRepository,
	authNsProvider repositories.AuthorizedNamespacesProvider,
) *OrgRepositoryProvider {
	return &OrgRepositoryProvider{
		orgRepo:        orgRepo,
		authNsProvider: authNsProvider,
	}
}

func (p *OrgRepositoryProvider) OrgRepoForRequest() (repositories.CFOrgRepository, error) {
	return repositories.NewOrgRepoAuthDecorator(p.orgRepo, p.authNsProvider), nil
}

type PrivilegedOrgRepositoryProvider struct {
	orgRepo repositories.CFOrgRepository
}

func NewPrivilegedOrg(orgRepo repositories.CFOrgRepository) *PrivilegedOrgRepositoryProvider {
	return &PrivilegedOrgRepositoryProvider{
		orgRepo: orgRepo,
	}
}

func (p *PrivilegedOrgRepositoryProvider) OrgRepoForRequest() (repositories.CFOrgRepository, error) {
	return p.orgRepo, nil
}
