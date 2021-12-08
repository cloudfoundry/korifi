package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ControllerConfig struct {
	KpackImageTag            string            `yaml:"kpackImageTag"`
	ClusterBuilderName       string            `yaml:"clusterBuilderName"`
	CFProcessDefaults        CFProcessDefaults `yaml:"cfProcessDefaults"`
	CFK8sControllerNamespace string            `yaml:"cfk8s_controller_namespace"`
	WorkloadsTLSSecretName   string            `yaml:"workloads_tls_secret_name"`
}

type CFProcessDefaults struct {
	MemoryMB           int64 `yaml:"memoryMB"`
	DefaultDiskQuotaMB int64 `yaml:"diskQuotaMB"`
}

func LoadFromPath(path string) (*ControllerConfig, error) {
	var config ControllerConfig

	items, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config dir %q: %w", path, err)
	}

	for _, item := range items {
		fileName := item.Name()
		if item.IsDir() || strings.HasPrefix(fileName, ".") {
			continue
		}

		configFile, err := os.Open(filepath.Join(path, fileName))
		if err != nil {
			return &config, fmt.Errorf("failed to open file: %w", err)
		}
		defer configFile.Close()

		decoder := yaml.NewDecoder(configFile)
		if err = decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("failed decoding %q: %w", item.Name(), err)
		}
	}

	return &config, nil
}

func (c ControllerConfig) WorkloadsTLSSecretNameWithNamespace() string {
	if c.WorkloadsTLSSecretName == "" {
		return ""
	}
	return filepath.Join(c.CFK8sControllerNamespace, c.WorkloadsTLSSecretName)
}
