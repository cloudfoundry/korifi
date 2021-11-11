package authorization

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
)

type CertInspector struct{}

func NewCertInspector() CertInspector {
	return CertInspector{}
}

func (c CertInspector) WhoAmI(_ context.Context, certPEMB64 string) (Identity, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(certPEMB64)
	if err != nil {
		return Identity{}, fmt.Errorf("failed to base64 decode cert")
	}

	pemBlock, _ := pem.Decode(pemBytes)
	if pemBlock == nil {
		return Identity{}, fmt.Errorf("failed to decode PEM")
	}

	cert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return Identity{}, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return Identity{
		Name: cert.Subject.CommonName,
		Kind: rbacv1.UserKind,
	}, nil
}
