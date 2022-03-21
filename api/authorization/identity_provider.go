package authorization

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
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
	if info.UserInfo != nil {
		fmt.Printf("info.UserInfo.GetName() = %+v\n", info.UserInfo.GetName())
		fmt.Printf("info.UserInfo.GetUID() = %+v\n", info.UserInfo.GetUID())
		fmt.Printf("info.UserInfo.GetGroups() = %+v\n", info.UserInfo.GetGroups())
		fmt.Printf("info.UserInfo.GetExtra() = %+v\n", info.UserInfo.GetExtra())
		return Identity{Name: info.UserInfo.GetName(), Kind: rbacv1.UserKind}, nil
	}

	if info.Token != "" {
		return p.tokenInspector.WhoAmI(ctx, info.Token)
	}

	if len(info.CertData) != 0 {
		return p.certInspector.WhoAmI(ctx, info.CertData)
	}

	return Identity{}, fmt.Errorf("invalid authorization info")
}
