package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type APIConfig struct {
	ServerURL                 string `yaml:"serverURL"`
	ServerPort                int    `yaml:"serverPort"`
	RootNamespace             string `yaml:"rootNamespace"`
	PackageRegistryBase       string `yaml:"packageRegistryBase"`
	PackageRegistrySecretName string `yaml:"packageRegistrySecretName"`

	DefaultLifecycleConfig DefaultLifecycleConfig `yaml:"defaultLifecycleConfig"`

	AuthEnabled  bool            `yaml:"authEnabled"`
	RoleMappings map[string]Role `yaml:"roleMappings"`
}

type Role struct {
	Name      string `yaml:"name"`
	Propagate bool   `yaml:"propagate"`
}

// DefaultLifecycleConfig contains default values of the Lifecycle block of CFApps and Builds created by the Shim
type DefaultLifecycleConfig struct {
	Type            string `yaml:"type"`
	Stack           string `yaml:"stack"`
	StagingMemoryMB int    `yaml:"stagingMemoryMB"`
	StagingDiskMB   int    `yaml:"stagingDiskMB"`
}

func LoadFromPath(path string) (*APIConfig, error) {
	var config APIConfig

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
