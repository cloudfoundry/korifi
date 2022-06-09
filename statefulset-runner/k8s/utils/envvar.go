package utils

import v1 "k8s.io/api/core/v1"

func MapToEnvVar(env map[string]string) []v1.EnvVar {
	envVars := []v1.EnvVar{}

	for k, v := range env {
		envVar := v1.EnvVar{Name: k, Value: v}
		envVars = append(envVars, envVar)
	}

	return envVars
}
