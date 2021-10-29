package provider

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"code.cloudfoundry.org/cf-k8s-api/repositories/authorization"
	"github.com/go-http-utils/headers"
)

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(ctx context.Context, authorizationHeader string) (authorization.Identity, error)
}

type OrgRepositoryProvider struct {
	orgRepo          repositories.CFOrgRepository
	authNsProvider   repositories.AuthorizedNamespacesProvider
	identityProvider IdentityProvider
}

func NewOrg(
	orgRepo repositories.CFOrgRepository,
	authNsProvider repositories.AuthorizedNamespacesProvider,
	identityProvider IdentityProvider) *OrgRepositoryProvider {
	return &OrgRepositoryProvider{
		orgRepo:          orgRepo,
		authNsProvider:   authNsProvider,
		identityProvider: identityProvider,
	}
}

func (p *OrgRepositoryProvider) OrgRepoForRequest(request *http.Request) (apis.CFOrgRepository, error) {
	identity, err := p.identityProvider.GetIdentity(request.Context(), request.Header.Get(headers.Authorization))
	if err != nil {
		return nil, err
	}

	return repositories.NewOrgRepoAuthDecorator(p.orgRepo, identity, p.authNsProvider), nil
}

type PrivilegedOrgRepositoryProvider struct {
	orgRepo repositories.CFOrgRepository
}

func NewPrivilegedOrg(orgRepo repositories.CFOrgRepository) *PrivilegedOrgRepositoryProvider {
	return &PrivilegedOrgRepositoryProvider{
		orgRepo: orgRepo,
	}
}

func (p *PrivilegedOrgRepositoryProvider) OrgRepoForRequest(_ *http.Request) (apis.CFOrgRepository, error) {
	return p.orgRepo, nil
}
