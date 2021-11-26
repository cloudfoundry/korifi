package repositories

import (
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildK8sClient(config *rest.Config) (k8sclient.Interface, error) {
	return k8sclient.NewForConfig(config)
}

type PrivilegedClientBuilder struct {
	config *rest.Config
}

func NewPrivilegedClientBuilder(config *rest.Config) PrivilegedClientBuilder {
	return PrivilegedClientBuilder{config: config}
}

func (c PrivilegedClientBuilder) BuildClient(_ string) (crclient.Client, error) {
	return crclient.New(c.config, crclient.Options{Scheme: scheme.Scheme})
}

type UserClientBuilder struct {
	config *rest.Config
}

func NewUserClientBuilder(config *rest.Config) UserClientBuilder {
	return UserClientBuilder{config: config}
}

func (c UserClientBuilder) BuildClient(authorizationHeader string) (crclient.Client, error) {
	if authorizationHeader == "" {
		return nil, authorization.NotAuthenticatedError{}
	}

	scheme, value, err := parseAuthorizationHeader(authorizationHeader)
	if err != nil {
		return nil, err
	}

	config := rest.AnonymousClientConfig(c.config)

	switch strings.ToLower(scheme) {
	case authorization.BearerScheme:
		config.BearerToken = value
	case authorization.CertScheme:
		pemBytes, decodeErr := base64.StdEncoding.DecodeString(value)
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to base64 decode auth header")
		}
		certBlock, rst := pem.Decode(pemBytes)
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
		return nil, fmt.Errorf("unknown auth header scheme %q", scheme)
	}

	// This does an API call within the controller-runtime code and is
	// sufficient to determine whether the auth is valid and accepted by the
	// cluster
	userClient, err := crclient.New(config, crclient.Options{})
	if err != nil {
		if apierrors.IsUnauthorized(err) {
			return nil, authorization.InvalidAuthError{}
		}
		return nil, err
	}

	return userClient, nil
}

func parseAuthorizationHeader(headerValue string) (string, string, error) {
	values := strings.Split(headerValue, " ")
	if len(values) != 2 {
		return "", "", errors.New("failed to parse authorization header")
	}
	return values[0], values[1], nil
}
