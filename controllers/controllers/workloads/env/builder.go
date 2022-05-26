package env

import (
	"context"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type vcapServicesPresenter struct {
	UserProvided []serviceDetails `json:"user-provided,omitempty"`
}

type serviceDetails struct {
	Label          string            `json:"label"`
	Name           string            `json:"name"`
	Tags           []string          `json:"tags"`
	InstanceGUID   string            `json:"instance_guid"`
	InstanceName   string            `json:"instance_name"`
	BindingGUID    string            `json:"binding_guid"`
	BindingName    *string           `json:"binding_name"`
	Credentials    map[string]string `json:"credentials"`
	SyslogDrainURL *string           `json:"syslog_drain_url"`
	VolumeMounts   []string          `json:"volume_mounts"`
}

type Builder struct {
	client workloads.CFClient
}

func NewBuilder(client workloads.CFClient) *Builder {
	return &Builder{client: client}
}

func (b *Builder) BuildEnv(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (map[string]string, error) {
	if cfApp.Spec.EnvSecretName == "" {
		return map[string]string{}, nil
	}

	appEnvSecret := corev1.Secret{}
	err := b.client.Get(ctx, types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Spec.EnvSecretName}, &appEnvSecret)
	if err != nil {
		return nil, fmt.Errorf("error when trying to fetch app env Secret %s/%s: %w", cfApp.Namespace, cfApp.Spec.EnvSecretName, err)
	}

	vcapServices, err := buildVcapServicesEnvValue(ctx, b.client, cfApp)
	if err != nil {
		return nil, err
	}

	updatedSecret := appEnvSecret.DeepCopy()
	if updatedSecret.Data == nil {
		updatedSecret.Data = map[string][]byte{}
	}
	updatedSecret.Data["VCAP_SERVICES"] = []byte(vcapServices)
	err = b.client.Patch(ctx, updatedSecret, client.MergeFrom(&appEnvSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to patch app env secret: %w", err)
	}

	return fromSecret(updatedSecret), nil
}

func fromSecret(secret *corev1.Secret) map[string]string {
	convertedMap := make(map[string]string)
	for k, v := range secret.Data {
		convertedMap[k] = string(v)
	}
	return convertedMap
}

func fromServiceBinding(
	serviceBinding korifiv1alpha1.CFServiceBinding,
	serviceInstance korifiv1alpha1.CFServiceInstance,
	serviceBindingSecret corev1.Secret,
) serviceDetails {
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

	return serviceDetails{
		Label:          "user-provided",
		Name:           serviceName,
		Tags:           tags,
		InstanceGUID:   serviceInstance.Name,
		InstanceName:   serviceInstance.Spec.DisplayName,
		BindingGUID:    serviceBinding.Name,
		BindingName:    bindingName,
		Credentials:    fromSecret(&serviceBindingSecret),
		SyslogDrainURL: nil,
		VolumeMounts:   []string{},
	}
}

func buildVcapServicesEnvValue(ctx context.Context, k8sClient workloads.CFClient, cfApp *korifiv1alpha1.CFApp) (string, error) {
	serviceBindings := &korifiv1alpha1.CFServiceBindingList{}
	err := k8sClient.List(ctx, serviceBindings,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name},
	)
	if err != nil {
		return "", fmt.Errorf("error listing CFServiceBindings: %w", err)
	}

	if len(serviceBindings.Items) == 0 {
		return "{}", nil
	}

	serviceEnvs := []serviceDetails{}
	for _, currentServiceBinding := range serviceBindings.Items {
		var serviceEnv serviceDetails
		serviceEnv, err = buildSingleServiceEnv(ctx, k8sClient, currentServiceBinding)
		if err != nil {
			return "", err
		}

		serviceEnvs = append(serviceEnvs, serviceEnv)
	}

	toReturn, err := json.Marshal(vcapServicesPresenter{
		UserProvided: serviceEnvs,
	})
	if err != nil {
		return "", err
	}

	return string(toReturn), nil
}

func buildSingleServiceEnv(ctx context.Context, k8sClient workloads.CFClient, serviceBinding korifiv1alpha1.CFServiceBinding) (serviceDetails, error) {
	if serviceBinding.Status.Binding.Name == "" {
		return serviceDetails{}, fmt.Errorf("service binding secret name is empty")
	}

	serviceInstance := korifiv1alpha1.CFServiceInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Spec.Service.Name}, &serviceInstance)
	if err != nil {
		return serviceDetails{}, fmt.Errorf("error fetching CFServiceInstance: %w", err)
	}

	secret := corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Status.Binding.Name}, &secret)
	if err != nil {
		return serviceDetails{}, fmt.Errorf("error fetching CFServiceBinding Secret: %w", err)
	}

	return fromServiceBinding(serviceBinding, serviceInstance, secret), nil
}
