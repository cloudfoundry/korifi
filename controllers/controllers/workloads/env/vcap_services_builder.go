package env

import (
	"context"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VCAPServicesEnvValueBuilder struct {
	k8sClient client.Client
}

func NewVCAPServicesEnvValueBuilder(k8sClient client.Client) *VCAPServicesEnvValueBuilder {
	return &VCAPServicesEnvValueBuilder{k8sClient: k8sClient}
}

func (b *VCAPServicesEnvValueBuilder) BuildEnvValue(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (map[string][]byte, error) {
	if len(cfApp.Status.ActualServiceBindingRefs) == 0 {
		return map[string][]byte{"VCAP_SERVICES": []byte("{}")}, nil
	}

	serviceEnvs := VCAPServices{}
	for _, currentServiceBinding := range cfApp.Status.ActualServiceBindingRefs {
		serviceEnv, serviceLabel, err := buildSingleServiceEnv(ctx, b.k8sClient, currentServiceBinding, cfApp.Namespace)
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

func buildSingleServiceEnv(ctx context.Context, k8sClient client.Client, serviceBinding korifiv1alpha1.ActualServiceBindingRef, namespace string) (ServiceDetails, string, error) {
	serviceInstance := korifiv1alpha1.CFServiceInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceBinding.ServiceGUID}, &serviceInstance)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error fetching CFServiceInstance: %w", err)
	}

	serviceLabel := string(serviceInstance.Spec.Type)

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      serviceBinding.EnvSecretRef,
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
	serviceBinding korifiv1alpha1.ActualServiceBindingRef,
	serviceInstance korifiv1alpha1.CFServiceInstance,
	credentialsSecret *corev1.Secret,
	serviceLabel string,
) (ServiceDetails, error) {
	var serviceName string
	var bindingName *string

	if serviceBinding.DisplayName != nil {
		serviceName = *serviceBinding.DisplayName
		bindingName = serviceBinding.DisplayName
	} else {
		serviceName = serviceInstance.Spec.DisplayName
		bindingName = nil
	}

	tags := serviceInstance.Spec.Tags
	if tags == nil {
		tags = []string{}
	}

	creds := map[string]any{}
	err := credentials.GetCredentials(credentialsSecret, &creds)
	if err != nil {
		return ServiceDetails{}, fmt.Errorf("failed to get credentials for service binding %q: %w", serviceBinding.GUID, err)
	}

	return ServiceDetails{
		Label:          serviceLabel,
		Name:           serviceName,
		Tags:           tags,
		InstanceGUID:   serviceInstance.Name,
		InstanceName:   serviceInstance.Spec.DisplayName,
		BindingGUID:    serviceBinding.GUID,
		BindingName:    bindingName,
		Credentials:    creds,
		SyslogDrainURL: nil,
		VolumeMounts:   []string{},
	}, nil
}
