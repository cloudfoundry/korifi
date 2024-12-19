package authorization

import (
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	k8sclient "k8s.io/client-go/kubernetes"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"k8s.io/client-go/rest"
)

type UserClientsetFactory interface {
	BuildClientset(Info) (k8sclient.Interface, error)
}

type UnprivilegedClientsetFactory struct {
	config *rest.Config
}

func NewUnprivilegedClientsetFactory(config *rest.Config) UnprivilegedClientsetFactory {
	return UnprivilegedClientsetFactory{
		config: rest.AnonymousClientConfig(rest.CopyConfig(config)),
	}
}

func (f UnprivilegedClientsetFactory) BuildClientset(authInfo Info) (k8sclient.Interface, error) {
	config := rest.CopyConfig(f.config)

	switch strings.ToLower(authInfo.Scheme()) {
	case BearerScheme:
		config.BearerToken = authInfo.Token

	case CertScheme:
		certBlock, rst := pem.Decode(authInfo.CertData)
		if certBlock == nil {
			return nil, fmt.Errorf("failed to decode cert PEM")
		}

		keyBlock, _ := pem.Decode(rst)
		if keyBlock == nil {
			return nil, fmt.Errorf("failed to decode key PEM")
		}

		config.CertData = pem.EncodeToMemory(certBlock)
		config.KeyData = pem.EncodeToMemory(keyBlock)

	default:
		return nil, apierrors.NewNotAuthenticatedError(errors.New("unsupported Authorization header scheme"))
	}

	userK8sClient, err := k8sclient.NewForConfig(config)
	if err != nil {
		return nil, apierrors.FromK8sError(err, "")
	}

	return userK8sClient, nil
}
