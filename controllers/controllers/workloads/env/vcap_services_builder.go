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
	if serviceBinding.Status.Credentials.Name == "" {
		return ServiceDetails{}, "", fmt.Errorf("credentials secret name not set for service binding %q", serviceBinding.Name)
	}

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

	serviceLabel, err := b.getServiceLabel(ctx, serviceInstance)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("failed to determine service label: %w", err)
	}

	tags, err := b.getServiceTags(ctx, serviceInstance)
	if err != nil {
		return ServiceDetails{}, "", fmt.Errorf("failed to determine service tags: %w", err)
	}

	serviceDetails, err := b.fromServiceBinding(ctx, serviceBinding, serviceInstance, credentialsSecret, serviceLabel, tags)
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

	serviceOffering, err := b.getServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingLabel])
	if err != nil {
		return "", err
	}

	return serviceOffering.Spec.Name, nil
}

func (b *VCAPServicesEnvValueBuilder) getServiceTags(ctx context.Context, cfServiceInstance korifiv1alpha1.CFServiceInstance) ([]string, error) {
	tags := cfServiceInstance.Spec.Tags
	if tags == nil {
		tags = []string{}
	}

	if cfServiceInstance.Spec.Type == korifiv1alpha1.ManagedType {
		servicePlan, err := b.getServicePlan(ctx, cfServiceInstance.Spec.ServicePlanGUID)
		if err != nil {
			return nil, err
		}

		serviceOffering, err := b.getServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingLabel])
		if err != nil {
			return nil, err
		}

		tags = append(tags, serviceOffering.Spec.Tags...)
	}

	return tags, nil
}

func (b *VCAPServicesEnvValueBuilder) fromServiceBinding(
	ctx context.Context,
	serviceBinding korifiv1alpha1.CFServiceBinding,
	serviceInstance korifiv1alpha1.CFServiceInstance,
	credentialsSecret *corev1.Secret,
	serviceLabel string,
	tags []string,
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

	serviceCredentials, err := credentials.GetCredentials(credentialsSecret)
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
		Credentials:    serviceCredentials,
		SyslogDrainURL: nil,
		VolumeMounts:   []string{},
	}, nil
}
