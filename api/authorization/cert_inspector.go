package authorization

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CertInspector struct {
	restConfig *rest.Config
}

func NewCertInspector(restConfig *rest.Config) *CertInspector {
	return &CertInspector{
		restConfig: restConfig,
	}
}

func (c *CertInspector) WhoAmI(ctx context.Context, certPEM []byte) (Identity, error) {
	certBlock, rst := pem.Decode(certPEM)
	if certBlock == nil {
		return Identity{}, apierrors.NewInvalidAuthError(errors.New("failed to decode cert PEM"))
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return Identity{}, apierrors.NewInvalidAuthError(fmt.Errorf("failed to parse certificate: %w", err))
	}

	keyBlock, _ := pem.Decode(rst)
	if keyBlock == nil {
		return Identity{}, apierrors.NewInvalidAuthError(errors.New("failed to decode key PEM"))
	}

	config := rest.AnonymousClientConfig(c.restConfig)
	config.CertData = pem.EncodeToMemory(certBlock)
	config.KeyData = pem.EncodeToMemory(keyBlock)

	// This does an API call within the controller-runtime code and is
	// sufficient to determine whether the certificate is valid and accepted by
	// the cluster
	_, err = client.New(config, client.Options{})
	if err != nil {
		return Identity{}, apierrors.FromK8sError(err, "")
	}

	return Identity{
		Name: cert.Subject.CommonName,
		Kind: rbacv1.UserKind,
	}, nil
}
