package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ControllerConfig struct {
	CFRootNamespace           string `yaml:"cfRootNamespace"`
	ClusterBuilderName        string `yaml:"clusterBuilderName"`
	BuilderServiceAccount     string `yaml:"builderServiceAccount"`
	ContainerRepositoryPrefix string `yaml:"containerRepositoryPrefix"`
	ContainerRegistryType     string `yaml:"containerRegistryType"`
}

func LoadFromPath(path string) (*ControllerConfig, error) {
	var config ControllerConfig

	items, err := os.ReadDir(path)
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
