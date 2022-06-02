package env

import (
	"context"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type vcapServicesPresenter struct {
	UserProvided []ServiceDetails `json:"user-provided,omitempty"`
}

type ServiceDetails struct {
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

func (b *Builder) BuildEnv(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error) {
	var appEnvSecret, vcapServicesSecret corev1.Secret

	if cfApp.Spec.EnvSecretName != "" {
		err := b.client.Get(ctx, types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Spec.EnvSecretName}, &appEnvSecret)
		if err != nil {
			return nil, fmt.Errorf("error when trying to fetch app env Secret %s/%s: %w", cfApp.Namespace, cfApp.Spec.EnvSecretName, err)
		}
	}

	if cfApp.Status.VCAPServicesSecretName != "" {
		err := b.client.Get(ctx, types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Status.VCAPServicesSecretName}, &vcapServicesSecret)
		if err != nil {
			return nil, fmt.Errorf("error when trying to fetch app env Secret %s/%s: %w", cfApp.Namespace, cfApp.Status.VCAPServicesSecretName, err)
		}
	}

	// We explicitly order the vcapServicesSecret last so that its "VCAP_SERVICES" contents win
	return envVarsFromSecrets(appEnvSecret, vcapServicesSecret), nil
}

func (b *Builder) BuildVCAPServicesEnvValue(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (string, error) {
	serviceBindings := &korifiv1alpha1.CFServiceBindingList{}
	err := b.client.List(ctx, serviceBindings,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name},
	)
	if err != nil {
		return "", fmt.Errorf("error listing CFServiceBindings: %w", err)
	}

	if len(serviceBindings.Items) == 0 {
		return "{}", nil
	}

	serviceEnvs := []ServiceDetails{}
	for _, currentServiceBinding := range serviceBindings.Items {
		var serviceEnv ServiceDetails
		serviceEnv, err = buildSingleServiceEnv(ctx, b.client, currentServiceBinding)
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

func mapFromSecret(secret corev1.Secret) map[string]string {
	convertedMap := make(map[string]string)
	for k, v := range secret.Data {
		convertedMap[k] = string(v)
	}
	return convertedMap
}

func envVarsFromSecrets(secrets ...corev1.Secret) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	for _, secret := range secrets {
		for k := range secret.Data {
			envVars = append(envVars, corev1.EnvVar{
				Name: k,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: secret.Name},
						Key:                  k,
					},
				},
			})
		}
	}
	return envVars
}

func fromServiceBinding(
	serviceBinding korifiv1alpha1.CFServiceBinding,
	serviceInstance korifiv1alpha1.CFServiceInstance,
	serviceBindingSecret corev1.Secret,
) ServiceDetails {
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

	return ServiceDetails{
		Label:          "user-provided",
		Name:           serviceName,
		Tags:           tags,
		InstanceGUID:   serviceInstance.Name,
		InstanceName:   serviceInstance.Spec.DisplayName,
		BindingGUID:    serviceBinding.Name,
		BindingName:    bindingName,
		Credentials:    mapFromSecret(serviceBindingSecret),
		SyslogDrainURL: nil,
		VolumeMounts:   []string{},
	}
}

func buildSingleServiceEnv(ctx context.Context, k8sClient workloads.CFClient, serviceBinding korifiv1alpha1.CFServiceBinding) (ServiceDetails, error) {
	if serviceBinding.Status.Binding.Name == "" {
		return ServiceDetails{}, fmt.Errorf("service binding secret name is empty")
	}

	serviceInstance := korifiv1alpha1.CFServiceInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Spec.Service.Name}, &serviceInstance)
	if err != nil {
		return ServiceDetails{}, fmt.Errorf("error fetching CFServiceInstance: %w", err)
	}

	secret := corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: serviceBinding.Status.Binding.Name}, &secret)
	if err != nil {
		return ServiceDetails{}, fmt.Errorf("error fetching CFServiceBinding Secret: %w", err)
	}

	return fromServiceBinding(serviceBinding, serviceInstance, secret), nil
}
