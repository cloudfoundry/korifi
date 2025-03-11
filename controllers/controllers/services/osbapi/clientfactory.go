package osbapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name BrokerClient code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi.BrokerClient
type BrokerClient interface {
	Provision(context.Context, ProvisionPayload) (ProvisionResponse, error)
	Deprovision(context.Context, DeprovisionPayload) (ProvisionResponse, error)
	GetServiceInstanceLastOperation(context.Context, GetInstanceLastOperationRequest) (LastOperationResponse, error)
	GetCatalog(context.Context) (Catalog, error)
	Bind(context.Context, BindPayload) (BindResponse, error)
	Unbind(context.Context, UnbindPayload) (UnbindResponse, error)
	GetServiceBindingLastOperation(context.Context, GetBindingLastOperationRequest) (LastOperationResponse, error)
	GetServiceBinding(ctx context.Context, payload BindPayload) (BindingResponse, error)
}

//counterfeiter:generate -o fake -fake-name BrokerClientFactory code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi.BrokerClientFactory
type BrokerClientFactory interface {
	CreateClient(context.Context, *korifiv1alpha1.CFServiceBroker) (BrokerClient, error)
}

type ClientFactory struct {
	k8sClient            client.Client
	trustInsecureBrokers bool
}

func NewClientFactory(k8sClient client.Client, trustInsecureBrokers bool) *ClientFactory {
	return &ClientFactory{
		k8sClient:            k8sClient,
		trustInsecureBrokers: trustInsecureBrokers,
	}
}

func (f *ClientFactory) CreateClient(ctx context.Context, cfServiceBroker *korifiv1alpha1.CFServiceBroker) (BrokerClient, error) {
	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBroker.Namespace,
			Name:      cfServiceBroker.Spec.Credentials.Name,
		},
	}

	err := f.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return nil, err
	}

	creds := map[string]string{}
	err = json.Unmarshal(credentialsSecret.Data[tools.CredentialsSecretKey], &creds)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal broker credentials secret: %w", err)
	}

	return NewClient(
		Broker{
			URL:      cfServiceBroker.Spec.URL,
			Username: creds["username"],
			Password: creds["password"],
		},
		&http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: f.trustInsecureBrokers}, //#nosec G402
		}},
	), nil
}
