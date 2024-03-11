package env

import (
	"context"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const UserProvided = "user-provided"

type VCAPServicesEnvValueBuilder struct {
	k8sClient     client.Client
	rootNamespace string
}

func NewVCAPServicesEnvValueBuilder(k8sClient client.Client, rootNamespace string) *VCAPServicesEnvValueBuilder {
	return &VCAPServicesEnvValueBuilder{
		k8sClient:     k8sClient,
		rootNamespace: rootNamespace,
	}
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
		serviceEnv, serviceLabel, err = b.buildSingleServiceEnv(ctx, b.k8sClient, currentServiceBinding)
		if err != nil {
			return nil, err
		}

		if _, ok := serviceEnvs[serviceEnv.Label]; !ok {
			serviceEnvs[serviceEnv.Label] = []ServiceDetails{}
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

func (b *VCAPServicesEnvValueBuilder) getServicePlan(ctx context.Context, servicePlanGuid string) (*korifiv1alpha1.CFServicePlan, error) {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.rootNamespace,
			Name:      servicePlanGuid,
		},
	}

	err := b.k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)
	if err != nil {
		return nil, fmt.Errorf("failed to get service plan :%q: %w", servicePlanGuid, err)
	}

	return servicePlan, nil
}

func (b *VCAPServicesEnvValueBuilder) getServiceOffering(ctx context.Context, serviceOfferingGuid string) (*korifiv1alpha1.CFServiceOffering, error) {
	serviceOffering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.rootNamespace,
			Name:      serviceOfferingGuid,
		},
	}
	err := b.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)
	if err != nil {
		return nil, fmt.Errorf("failed to get service offering %q: %w", serviceOfferingGuid, err)
	}

	return serviceOffering, nil
}

func (b *VCAPServicesEnvValueBuilder) buildSingleServiceEnv(ctx context.Context, k8sClient client.Client, serviceBinding korifiv1alpha1.CFServiceBinding) (ServiceDetails, string, error) {
	if serviceBinding.Status.Binding.Name == "" {
		return ServiceDetails{}, "", fmt.Errorf("secret name not set for service binding %q", serviceBinding.Name)
	}

	serviceInstance := korifiv1alpha1.CFServiceInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Spec.Service.Name}, &serviceInstance)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error fetching CFServiceInstance: %w", err)
	}

	secret := corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Status.Binding.Name}, &secret)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error fetching CFServiceBinding Secret: %w", err)
	}

	serviceLabel, err := b.getServiceLabel(ctx, serviceInstance)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("failed to determine service label: %w", err)
	}

	serviceDetails, err := b.fromServiceBinding(ctx, serviceBinding, serviceInstance, secret, serviceLabel)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("error creating service details: %w", err)
	}
	return serviceDetails, serviceLabel, nil
}

func (b *VCAPServicesEnvValueBuilder) getServiceLabel(ctx context.Context, cfServiceInstance korifiv1alpha1.CFServiceInstance) (string, error) {
	if cfServiceInstance.Spec.Type == korifiv1alpha1.UserProvidedType {
		if cfServiceInstance.Spec.ServiceLabel != nil && *cfServiceInstance.Spec.ServiceLabel != "" {
			return *cfServiceInstance.Spec.ServiceLabel, nil
		}

		return korifiv1alpha1.UserProvidedType, nil
	}

	servicePlan, err := b.getServicePlan(ctx, cfServiceInstance.Spec.ServicePlanGUID)
	if err != nil {
		return "", err
	}

	serviceOffering, err := b.getServiceOffering(ctx, servicePlan.Spec.Relationships.ServiceOfferingGUID)
	if err != nil {
		return "", err
	}

	return serviceOffering.Spec.OfferingName, nil
}

func (b *VCAPServicesEnvValueBuilder) fromServiceBinding(
	ctx context.Context,
	serviceBinding korifiv1alpha1.CFServiceBinding,
	serviceInstance korifiv1alpha1.CFServiceInstance,
	serviceBindingSecret corev1.Secret,
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

	var credentials map[string]any
	if serviceInstance.Spec.Type == korifiv1alpha1.ManagedType {
		managedTags, err := tagsFromManagedBindingSecret(serviceBindingSecret)
		if err != nil {
			return ServiceDetails{}, fmt.Errorf("failed to get tags from managed service binding secret: %w", err)
		}
		tags = append(tags, managedTags...)

		credentials, err = credentialsFromManagedBindingSecret(serviceBindingSecret)
		if err != nil {
			return ServiceDetails{}, fmt.Errorf("failed to get credentials from managed service binding secret: %w", err)
		}
	} else {
		credentials = credentialsFromUserProvidedBindingSecret(serviceBindingSecret)
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

func tagsFromManagedBindingSecret(serviceBindingSecret corev1.Secret) ([]string, error) {
	tags := []string{}
	bindingTagBytes, ok := serviceBindingSecret.Data["tags"]
	if ok {
		var bindingTags []string
		if err := json.Unmarshal(bindingTagBytes, &bindingTags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal binding secret tags: %w", err)
		}

		tags = append(tags, bindingTags...)
	}

	return tags, nil
}

func credentialsFromManagedBindingSecret(serviceBindingSecret corev1.Secret) (map[string]any, error) {
	credentials := map[string]any{}
	for k, v := range serviceBindingSecret.Data {
		var credValue any
		err := json.Unmarshal(v, &credValue)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal binding secret key %q: %w", k, err)
		}
		credentials[k] = credValue

	}

	return credentials, nil
}

func credentialsFromUserProvidedBindingSecret(secret corev1.Secret) map[string]any {
	convertedMap := make(map[string]any)
	for k, v := range secret.Data {
		convertedMap[k] = string(v)
	}
	return convertedMap
}
