package env

import (
	"context"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const UserProvided = "user-provided"

type VCAPServicesEnvValueBuilder struct {
	k8sClient client.Client
}

func NewVCAPServicesEnvValueBuilder(k8sClient client.Client) *VCAPServicesEnvValueBuilder {
	return &VCAPServicesEnvValueBuilder{k8sClient: k8sClient}
}

func (b *VCAPServicesEnvValueBuilder) BuildEnvValue(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (map[string][]byte, error) {
	serviceBindings := &korifiv1alpha1.CFServiceBindingList{}
	err := b.k8sClient.List(ctx, serviceBindings,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name},
	)
	if err != nil {
		return nil, fmt.Errorf("error listing CFServiceBindings: %w", err)
	}

	if len(serviceBindings.Items) == 0 {
		return map[string][]byte{"VCAP_SERVICES": []byte("{}")}, nil
	}

	serviceEnvs := VCAPServices{}
	for _, currentServiceBinding := range serviceBindings.Items {
		// If finalizing do not append
		if !currentServiceBinding.DeletionTimestamp.IsZero() {
			continue
		}

		var serviceEnv ServiceDetails
		var serviceLabel string
		serviceEnv, serviceLabel, err = buildSingleServiceEnv(ctx, b.k8sClient, currentServiceBinding)
		if err != nil {
			return nil, err
		}

		serviceEnvs[serviceLabel] = append(serviceEnvs[serviceLabel], serviceEnv)
	}

	jsonVal, err := json.Marshal(serviceEnvs)
	if err != nil {
		return nil, err
	}

	return map[string][]byte{
		"VCAP_SERVICES": jsonVal,
	}, nil
}

func buildSingleServiceEnv(ctx context.Context, k8sClient client.Client, serviceBinding korifiv1alpha1.CFServiceBinding) (ServiceDetails, string, error) {
	if serviceBinding.Status.Credentials.Name == "" {
		return ServiceDetails{}, "", fmt.Errorf("credentials secret name not set for service binding %q", serviceBinding.Name)
	}

	serviceLabel := UserProvided

	serviceInstance := korifiv1alpha1.CFServiceInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Spec.Service.Name}, &serviceInstance)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error fetching CFServiceInstance: %w", err)
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serviceBinding.Namespace,
			Name:      serviceBinding.Status.Credentials.Name,
		},
	}
	err = k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error fetching CFServiceBinding Secret: %w", err)
	}

	if serviceInstance.Spec.ServiceLabel != nil && *serviceInstance.Spec.ServiceLabel != "" {
		serviceLabel = *serviceInstance.Spec.ServiceLabel
	}

	serviceDetails, err := fromServiceBinding(serviceBinding, serviceInstance, credentialsSecret, serviceLabel)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error fetching CFServiceBinding details: %w", err)
	}

	return serviceDetails, serviceLabel, nil
}

func fromServiceBinding(
	serviceBinding korifiv1alpha1.CFServiceBinding,
	serviceInstance korifiv1alpha1.CFServiceInstance,
	credentialsSecret *corev1.Secret,
	serviceLabel string,
) (ServiceDetails, error) {
	var serviceName string
	var bindingName *string

	if serviceBinding.Spec.DisplayName != nil {
		serviceName = *serviceBinding.Spec.DisplayName
		bindingName = serviceBinding.Spec.DisplayName
	} else {
		serviceName = serviceInstance.Spec.DisplayName
		bindingName = nil
	}

	tags := serviceInstance.Spec.Tags
	if tags == nil {
		tags = []string{}
	}

	credentials, err := credentials.GetCredentials(credentialsSecret)
	if err != nil {
		return ServiceDetails{}, fmt.Errorf("failed to get credentials for service binding %q: %w", serviceBinding.Name, err)
	}

	return ServiceDetails{
		Label:          serviceLabel,
		Name:           serviceName,
		Tags:           tags,
		InstanceGUID:   serviceInstance.Name,
		InstanceName:   serviceInstance.Spec.DisplayName,
		BindingGUID:    serviceBinding.Name,
		BindingName:    bindingName,
		Credentials:    credentials,
		SyslogDrainURL: nil,
		VolumeMounts:   []string{},
	}, nil
}
