package repositories

import (
	"encoding/pem"
	"fmt"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierr"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UserK8sClientFactory interface {
	BuildClient(authorization.Info) (client.WithWatch, error)
}

type UnprivilegedClientFactory struct {
	config *rest.Config
}

func NewUnprivilegedClientFactory(config *rest.Config) UnprivilegedClientFactory {
	return UnprivilegedClientFactory{
		config: rest.AnonymousClientConfig(rest.CopyConfig(config)),
	}
}

func (f UnprivilegedClientFactory) BuildClient(authInfo authorization.Info) (client.WithWatch, error) {
	config := rest.CopyConfig(f.config)

	switch strings.ToLower(authInfo.Scheme()) {
	case authorization.BearerScheme:
		config.BearerToken = authInfo.Token

	case authorization.CertScheme:
		certBlock, rst := pem.Decode(authInfo.CertData)
		if certBlock == nil {
			return nil, apierr.NewInvalidAuthError(fmt.Errorf("failed to decode cert PEM"))
		}

		keyBlock, _ := pem.Decode(rst)
		if keyBlock == nil {
			return nil, apierr.NewInvalidAuthError(fmt.Errorf("failed to decode key PEM"))
		}

		config.CertData = pem.EncodeToMemory(certBlock)
		config.KeyData = pem.EncodeToMemory(keyBlock)

	default:
		return nil, apierr.NewInvalidAuthError(fmt.Errorf("did not send bearer or clientcert scheme in auth header"))
	}

	// This does an API call within the controller-runtime code and is
	// sufficient to determine whether the auth is valid and accepted by the
	// cluster
	unprivilegedClient, err := client.NewWithWatch(config, client.Options{})
	if err != nil {
		if k8serrors.IsUnauthorized(err) {
			return nil, apierr.NewInvalidAuthError(err)
		}
		return nil, err
	}

	return unprivilegedClient, nil
}

func NewPrivilegedClientFactory(config *rest.Config) PrivilegedClientFactory {
	return PrivilegedClientFactory{
		config: config,
	}
}

type PrivilegedClientFactory struct {
	config *rest.Config
}

func (f PrivilegedClientFactory) BuildClient(_ authorization.Info) (client.WithWatch, error) {
	return client.NewWithWatch(f.config, client.Options{Scheme: scheme.Scheme})
}
