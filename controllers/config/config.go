package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ControllerConfig struct {
	CFProcessDefaults           CFProcessDefaults `yaml:"cfProcessDefaults"`
	CFRootNamespace             string            `yaml:"cfRootNamespace"`
	PackageRegistrySecretName   string            `yaml:"packageRegistrySecretName"`
	TaskTTL                     string            `yaml:"taskTTL"`
	WorkloadsTLSSecretName      string            `yaml:"workloads_tls_secret_name"`
	WorkloadsTLSSecretNamespace string            `yaml:"workloads_tls_secret_namespace"`
}

type CFProcessDefaults struct {
	MemoryMB    int64 `yaml:"memoryMB"`
	DiskQuotaMB int64 `yaml:"diskQuotaMB"`
}

const defaultTaskTTL = 30 * 24 * time.Hour

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
	return filepath.Join(c.WorkloadsTLSSecretNamespace, c.WorkloadsTLSSecretName)
}

func (c ControllerConfig) ParseTaskTTL() (time.Duration, error) {
	if c.TaskTTL == "" {
		return defaultTaskTTL, nil
	}

	splitByDays := strings.Split(c.TaskTTL, "d")
	switch len(splitByDays) {
	case 1:
		return time.ParseDuration(c.TaskTTL)
	case 2:
		days, err := time.ParseDuration(splitByDays[0] + "h")
		if err != nil {
			return 0, errors.New("failed to parse " + c.TaskTTL)
		}

		var parsedDuration time.Duration = 0
		if splitByDays[1] != "" {
			parsedDuration, err = time.ParseDuration(splitByDays[1])
			if err != nil {
				return 0, errors.New("failed to parse " + c.TaskTTL)
			}
		}

		return days*24 + parsedDuration, nil
	default:
		return 0, errors.New("failed to parse " + c.TaskTTL)
	}
}
