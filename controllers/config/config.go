package config

import (
	"time"

	"code.cloudfoundry.org/korifi/tools"
)

type ControllerConfig struct {
	CFProcessDefaults         CFProcessDefaults `yaml:"cfProcessDefaults"`
	CFRootNamespace           string            `yaml:"cfRootNamespace"`
	PackageRegistrySecretName string            `yaml:"packageRegistrySecretName"`
	TaskTTL                   string            `yaml:"taskTTL"`
	BuilderName               string            `yaml:"builderName"`
	RunnerName                string            `yaml:"runnerName"`
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

func (c ControllerConfig) ParseTaskTTL() (time.Duration, error) {
	if c.TaskTTL == "" {
		return defaultTaskTTL, nil
	}

	return tools.ParseDuration(c.TaskTTL)
}
