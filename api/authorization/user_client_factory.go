package authorization

import (
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClientWrappingFunc func(client.WithWatch) client.WithWatch

//counterfeiter:generate -o fake -fake-name UserClientFactory . UserClientFactory
type UserClientFactory interface {
	BuildClient(Info) (client.WithWatch, error)
}

type UnprivilegedClientFactory struct {
	config   *rest.Config
	mapper   meta.RESTMapper
	wrappers []ClientWrappingFunc
	scheme   *runtime.Scheme
}

func NewUnprivilegedClientFactory(config *rest.Config, mapper meta.RESTMapper, scheme *runtime.Scheme) UnprivilegedClientFactory {
	return UnprivilegedClientFactory{
		config:   rest.AnonymousClientConfig(rest.CopyConfig(config)),
		mapper:   mapper,
		wrappers: []ClientWrappingFunc{},
		scheme:   scheme,
	}
}

func (f UnprivilegedClientFactory) WithWrappingFunc(wrapper ClientWrappingFunc) UnprivilegedClientFactory {
	f.wrappers = append(f.wrappers, wrapper)
	return f
}

func (f UnprivilegedClientFactory) BuildClient(authInfo Info) (client.WithWatch, error) {
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

	userClient, err := client.NewWithWatch(config, client.Options{
		Scheme: f.scheme,
		Mapper: f.mapper,
	})
	if err != nil {
		return nil, apierrors.FromK8sError(err, "")
	}

	for _, wrapper := range f.wrappers {
		userClient = wrapper(userClient)
	}

	return userClient, nil
}
