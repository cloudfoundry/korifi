package env

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/ports"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VCAPServices map[string][]ServiceDetails

type ServiceDetails struct {
	Label          string         `json:"label"`
	Name           string         `json:"name"`
	Tags           []string       `json:"tags"`
	InstanceGUID   string         `json:"instance_guid"`
	InstanceName   string         `json:"instance_name"`
	BindingGUID    string         `json:"binding_guid"`
	BindingName    *string        `json:"binding_name"`
	Credentials    map[string]any `json:"credentials"`
	SyslogDrainURL *string        `json:"syslog_drain_url"`
	VolumeMounts   []string       `json:"volume_mounts"`
}

type AppEnvBuilder struct {
	k8sClient client.Client
}

func NewAppEnvBuilder(k8sClient client.Client) *AppEnvBuilder {
	return &AppEnvBuilder{k8sClient: k8sClient}
}

func (b *AppEnvBuilder) Build(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error) {
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
	return sortEnvVars(envVarsFromSecrets(appEnvSecret, vcapServicesSecret, vcapApplicationSecret)), nil
}

func sortEnvVars(envVars []corev1.EnvVar) []corev1.EnvVar {
	slices.SortStableFunc(envVars, func(envVar1, envVar2 corev1.EnvVar) int {
		return strings.Compare(envVar1.Name, envVar2.Name)
	})

	return envVars
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

type ProcessEnvBuilder struct {
	appEnvBuilder *AppEnvBuilder
	k8sClient     client.Client
}

func NewProcessEnvBuilder(k8sClient client.Client) *ProcessEnvBuilder {
	return &ProcessEnvBuilder{
		appEnvBuilder: NewAppEnvBuilder(k8sClient),
		k8sClient:     k8sClient,
	}
}

func (b *ProcessEnvBuilder) Build(ctx context.Context, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess) ([]corev1.EnvVar, error) {
	env, err := b.appEnvBuilder.Build(ctx, cfApp)
	if err != nil {
		return nil, err
	}

	env = append(env,
		corev1.EnvVar{Name: "VCAP_APP_HOST", Value: "0.0.0.0"},
		corev1.EnvVar{Name: "MEMORY_LIMIT", Value: fmt.Sprintf("%dM", cfProcess.Spec.MemoryMB)},
	)

	portEnv, err := b.buildPortEnv(ctx, cfApp, cfProcess)
	if err != nil {
		return nil, err
	}
	env = append(env, portEnv...)

	return sortEnvVars(env), nil
}

func (b *ProcessEnvBuilder) buildPortEnv(ctx context.Context, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess) ([]corev1.EnvVar, error) {
	var cfRoutesForProcess korifiv1alpha1.CFRouteList
	err := b.k8sClient.List(ctx, &cfRoutesForProcess,
		client.InNamespace(cfProcess.Namespace),
		client.MatchingFields{shared.IndexRouteDestinationAppName: cfApp.Name},
	)
	if err != nil {
		return nil, err
	}

	processPorts := ports.FromRoutes(cfRoutesForProcess.Items, cfApp.Name, cfProcess.Spec.ProcessType)
	if len(processPorts) > 0 {
		portString := strconv.FormatInt(int64(processPorts[0]), 10)
		cfInstancePorts := fmt.Sprintf("[{\"internal\":%s}]", portString)
		return []corev1.EnvVar{
			{Name: "VCAP_APP_PORT", Value: portString},
			{Name: "PORT", Value: portString},
			{Name: "CF_INSTANCE_PORTS", Value: cfInstancePorts},
		}, nil
	}

	return nil, nil
}
