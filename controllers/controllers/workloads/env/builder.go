package env

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VCAPServices map[string][]ServiceDetails

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
	Plan           string            `json:"plan,omitempty"`
}

type WorkloadEnvBuilder struct {
	k8sClient client.Client
}

func NewWorkloadEnvBuilder(k8sClient client.Client) *WorkloadEnvBuilder {
	return &WorkloadEnvBuilder{k8sClient: k8sClient}
}

func (b *WorkloadEnvBuilder) BuildEnv(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error) {
	var appEnvSecret, vcapServicesSecret, vcapApplicationSecret corev1.Secret

	if cfApp.Spec.EnvSecretName != "" {
		err := b.k8sClient.Get(ctx, types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Spec.EnvSecretName}, &appEnvSecret)
		if err != nil {
			return nil, fmt.Errorf("error when trying to fetch app env Secret %s/%s: %w", cfApp.Namespace, cfApp.Spec.EnvSecretName, err)
		}
	}

	if cfApp.Status.VCAPServicesSecretName != "" {
		err := b.k8sClient.Get(ctx, types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Status.VCAPServicesSecretName}, &vcapServicesSecret)
		if err != nil {
			return nil, fmt.Errorf("error when trying to fetch vcap services secret %s/%s: %w", cfApp.Namespace, cfApp.Status.VCAPServicesSecretName, err)
		}
	}

	if cfApp.Status.VCAPApplicationSecretName != "" {
		err := b.k8sClient.Get(ctx, types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Status.VCAPApplicationSecretName}, &vcapApplicationSecret)
		if err != nil {
			return nil, fmt.Errorf("error when trying to fetch vcap application secret %s/%s: %w", cfApp.Namespace, cfApp.Status.VCAPApplicationSecretName, err)
		}
	}

	// We explicitly order the vcapServicesSecret last so that its "VCAP_*" contents win
	return envVarsFromSecrets(appEnvSecret, vcapServicesSecret, vcapApplicationSecret), nil
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
