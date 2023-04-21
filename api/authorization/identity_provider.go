package authorization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	BearerScheme  string = "bearer"
	CertScheme    string = "clientcert"
	UnknownScheme string = "unknown"
)

//counterfeiter:generate -o fake -fake-name TokenIdentityInspector . TokenIdentityInspector
//counterfeiter:generate -o fake -fake-name CertIdentityInspector . CertIdentityInspector

type Identity rbacv1.Subject

func NewIdentity(kind, name, namespace string) (Identity, error) {
	switch kind {
	case rbacv1.UserKind:
		return Identity{
			Kind:     rbacv1.UserKind,
			APIGroup: "rbac.authorization.k8s.io",
			Name:     name,
		}, nil
	case rbacv1.ServiceAccountKind:
		return Identity{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      name,
			Namespace: namespace,
		}, nil
	default:
		return Identity{}, errors.New("The rbacv1 SubjectKind provided is not supported")
	}
}

func (i *Identity) Hash() string {
	key := append([]byte(i.Name), []byte(i.Kind)...)
	hasher := sha256.New()
	return hex.EncodeToString(hasher.Sum(key))
}

func (i *Identity) IsSubject(subject rbacv1.Subject) (bool, error) {
	if i.Kind != subject.Kind {
		return false, nil
	}

	if i.Kind == "ServiceAccount" {
		if !HasServiceAccountPrefix(i.Name) {
			return false, fmt.Errorf("expected user identifier %q to have prefix %q", i.Name, serviceAccountNamePrefix)
		}
		identitySANS, identitySAName := ServiceAccountNSAndName(i.Name)
		return identitySAName == subject.Name && identitySANS == subject.Namespace, nil
	} else {
		return i.Name == subject.Name, nil
	}
}

func HasServiceAccountPrefix(idName string) bool {
	return strings.HasPrefix(idName, serviceAccountNamePrefix)
}

func ServiceAccountNSAndName(name string) (string, string) {
	nameSegments := strings.Split(name, ":")

	serviceAccountNS := nameSegments[2]
	serviceAccountName := nameSegments[3]

	return serviceAccountNS, serviceAccountName
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
