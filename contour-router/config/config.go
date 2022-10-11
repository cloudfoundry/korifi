package config

import (
	"path/filepath"

	"code.cloudfoundry.org/korifi/tools"
)

type ContourRouterConfig struct {
	WorkloadsTLSSecretName      string `yaml:"workloads_tls_secret_name"`
	WorkloadsTLSSecretNamespace string `yaml:"workloads_tls_secret_namespace"`
}

func (c ContourRouterConfig) WorkloadsTLSSecretNameWithNamespace() string {
	if c.WorkloadsTLSSecretName == "" {
		return ""
	}
	return filepath.Join(c.WorkloadsTLSSecretNamespace, c.WorkloadsTLSSecretName)
}

func LoadFromPath(path string) (*ContourRouterConfig, error) {
	var config ContourRouterConfig
	err := tools.LoadConfigInto(&config, path)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
