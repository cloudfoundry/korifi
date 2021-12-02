package repositories

import (
	"encoding/pem"
	"fmt"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UserK8sClientFactory interface {
	BuildClient(authorization.Info) (client.Client, error)
}

type UnprivilegedClientFactory struct {
	config *rest.Config
}

func NewUnprivilegedClientFactory(config *rest.Config) UnprivilegedClientFactory {
	return UnprivilegedClientFactory{
		config: rest.AnonymousClientConfig(rest.CopyConfig(config)),
	}
}

func (f UnprivilegedClientFactory) BuildClient(authInfo authorization.Info) (client.Client, error) {
	switch strings.ToLower(authInfo.Scheme()) {
	case authorization.BearerScheme:
		f.config.BearerToken = authInfo.Token

	case authorization.CertScheme:
		certBlock, rst := pem.Decode(authInfo.CertData)
		if certBlock == nil {
			return nil, fmt.Errorf("failed to decode cert PEM")
		}

		keyBlock, _ := pem.Decode(rst)
		if keyBlock == nil {
			return nil, fmt.Errorf("failed to decode key PEM")
		}

		f.config.CertData = pem.EncodeToMemory(certBlock)
		f.config.KeyData = pem.EncodeToMemory(keyBlock)

	default:
		return nil, authorization.NotAuthenticatedError{}
	}

	// This does an API call within the controller-runtime code and is
	// sufficient to determine whether the auth is valid and accepted by the
	// cluster
	userClient, err := client.New(f.config, client.Options{})
	if err != nil {
		if k8serrors.IsUnauthorized(err) {
			return nil, authorization.InvalidAuthError{}
		}
		return nil, err
	}

	return userClient, nil
}

func NewPrivilegedClientFactory(config *rest.Config) PrivilegedClientFactory {
	return PrivilegedClientFactory{
		config: config,
	}
}

type PrivilegedClientFactory struct {
	config *rest.Config
}

func (f PrivilegedClientFactory) BuildClient(_ authorization.Info) (client.Client, error) {
	return client.New(f.config, client.Options{Scheme: scheme.Scheme})
}
