package provider

import (
	"context"
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(ctx context.Context, authInfo authorization.Info) (authorization.Identity, error)
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

func (p *OrgRepositoryProvider) OrgRepoForRequest(request *http.Request) (repositories.CFOrgRepository, error) {
	authInfo, ok := authorization.InfoFromContext(request.Context())
	if !ok {
		return nil, errors.New("no authorization info in the request context")
	}

	identity, err := p.identityProvider.GetIdentity(request.Context(), *authInfo)
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

func (p *PrivilegedOrgRepositoryProvider) OrgRepoForRequest(_ *http.Request) (repositories.CFOrgRepository, error) {
	return p.orgRepo, nil
}
