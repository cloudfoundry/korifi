package config

import (
	"path/filepath"
	"time"

	"code.cloudfoundry.org/korifi/tools"
)

type ControllerConfig struct {
	CFProcessDefaults           CFProcessDefaults `yaml:"cfProcessDefaults"`
	CFRootNamespace             string            `yaml:"cfRootNamespace"`
	ContainerRegistrySecretName string            `yaml:"containerRegistrySecretName"`
	TaskTTL                     string            `yaml:"taskTTL"`
	WorkloadsTLSSecretName      string            `yaml:"workloads_tls_secret_name"`
	WorkloadsTLSSecretNamespace string            `yaml:"workloads_tls_secret_namespace"`
	BuilderName                 string            `yaml:"builderName"`
	RunnerName                  string            `yaml:"runnerName"`
	NamespaceLabels             map[string]string `yaml:"namespaceLabels"`
}

type CFProcessDefaults struct {
	MemoryMB    int64  `yaml:"memoryMB"`
	DiskQuotaMB int64  `yaml:"diskQuotaMB"`
	Timeout     *int64 `yaml:"timeout"`
}

const (
	defaultTaskTTL       = 30 * 24 * time.Hour
	defaultTimeout int64 = 60
)

func LoadFromPath(path string) (*ControllerConfig, error) {
	var config ControllerConfig
	err := tools.LoadConfigInto(&config, path)
	if err != nil {
		return nil, err
	}

	if config.CFProcessDefaults.Timeout == nil {
		config.CFProcessDefaults.Timeout = tools.PtrTo(defaultTimeout)
	}

	return &config, nil
}

func (c ControllerConfig) WorkloadsTLSSecretNameWithNamespace() string {
	if c.WorkloadsTLSSecretName == "" {
		return ""
	}
	return filepath.Join(c.WorkloadsTLSSecretNamespace, c.WorkloadsTLSSecretName)
}

func (c ControllerConfig) ParseTaskTTL() (time.Duration, error) {
	if c.TaskTTL == "" {
		return defaultTaskTTL, nil
	}

	return tools.ParseDuration(c.TaskTTL)
}
