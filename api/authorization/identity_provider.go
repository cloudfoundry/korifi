package authorization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func (i *Identity) Hash() string {
	key := append([]byte(i.Name), []byte(i.Kind)...)
	hasher := sha256.New()
	return hex.EncodeToString(hasher.Sum(key))
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

	return Identity{}, fmt.Errorf("invalid authorization info")
}
