package provider

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
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

func (p *OrgRepositoryProvider) OrgRepoForRequest(request *http.Request) (repositories.CFOrgRepository, error) {
	authInfo, ok := authorization.InfoFromContext(request.Context())
	if !ok {
		return nil, errors.New("no authorization info in the request context")
	}

	return repositories.NewOrgRepoAuthDecorator(p.orgRepo, authInfo, p.authNsProvider), nil
}

type PrivilegedOrgRepositoryProvider struct {
	orgRepo repositories.CFOrgRepository
}

func NewPrivilegedOrg(orgRepo repositories.CFOrgRepository) *PrivilegedOrgRepositoryProvider {
	return &PrivilegedOrgRepositoryProvider{
		orgRepo: orgRepo,
	}
}

func (p *PrivilegedOrgRepositoryProvider) OrgRepoForRequest(_ *http.Request) (repositories.CFOrgRepository, error) {
	return p.orgRepo, nil
}
