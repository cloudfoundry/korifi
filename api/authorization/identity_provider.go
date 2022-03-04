package authorization

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	BearerScheme   string = "bearer"
	CertScheme     string = "clientcert"
	UsernameScheme string = "username"
	UnknownScheme  string = "unknown"
)

//counterfeiter:generate -o fake -fake-name TokenIdentityInspector . TokenIdentityInspector
//counterfeiter:generate -o fake -fake-name CertIdentityInspector . CertIdentityInspector

type Identity struct {
	Name string
	Kind string
}

type TokenIdentityInspector interface {
	WhoAmI(context.Context, string) (Identity, error)
}

type CertIdentityInspector interface {
	WhoAmI(context.Context, []byte) (Identity, error)
}

type CertTokenIdentityProvider struct {
	tokenInspector TokenIdentityInspector
	certInspector  CertIdentityInspector
}

func NewCertTokenIdentityProvider(tokenInspector TokenIdentityInspector, certInspector CertIdentityInspector) *CertTokenIdentityProvider {
	return &CertTokenIdentityProvider{
		tokenInspector: tokenInspector,
		certInspector:  certInspector,
	}
}

func (p *CertTokenIdentityProvider) GetIdentity(ctx context.Context, info Info) (Identity, error) {
	if info.Token != "" {
		return p.tokenInspector.WhoAmI(ctx, info.Token)
	}

	if len(info.CertData) != 0 {
		return p.certInspector.WhoAmI(ctx, info.CertData)
	}

	if info.Username != "" {
		return Identity{Name: info.Username, Kind: rbacv1.UserKind}, nil
	}

	return Identity{}, fmt.Errorf("invalid authorization info")
}
