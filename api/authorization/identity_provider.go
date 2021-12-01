package authorization

import (
	"context"
	"fmt"
)

const (
	BearerScheme  string = "bearer"
	CertScheme    string = "clientcert"
	UnknownScheme string = "unknown"
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

type IdentityProvider struct {
	tokenInspector TokenIdentityInspector
	certInspector  CertIdentityInspector
}

func NewIdentityProvider(tokenInspector TokenIdentityInspector, certInspector CertIdentityInspector) *IdentityProvider {
	return &IdentityProvider{
		tokenInspector: tokenInspector,
		certInspector:  certInspector,
	}
}

func (p *IdentityProvider) GetIdentity(ctx context.Context, info Info) (Identity, error) {
	if info.Token != "" {
		return p.tokenInspector.WhoAmI(ctx, info.Token)
	}

	if len(info.CertData) != 0 {
		return p.certInspector.WhoAmI(ctx, info.CertData)
	}

	return Identity{}, fmt.Errorf("invalid authorization info")
}
