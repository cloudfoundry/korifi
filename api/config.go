package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerURL                 string `yaml:"serverURL"`
	ServerPort                int    `yaml:"serverPort"`
	RootNamespace             string `yaml:"rootNamespace"`
	PackageRegistryBase       string `yaml:"packageRegistryBase"`
	PackageRegistrySecretName string `yaml:"packageRegistrySecretName"`

	DefaultLifecycleConfig DefaultLifecycleConfig `yaml:"defaultLifecycleConfig"`

	AuthEnabled bool `yaml:"authEnabled"`
}

// DefaultLifecycleConfig contains default values of the Lifecycle block of CFApps and Builds created by the Shim
type DefaultLifecycleConfig struct {
	Type            string `yaml:"type"`
	Stack           string `yaml:"stack"`
	StagingMemoryMB int    `yaml:"stagingMemoryMB"`
	StagingDiskMB   int    `yaml:"stagingDiskMB"`
}

func LoadFromPath(path string) (*Config, error) {
	var config Config
	configFile, err := os.Open(path)
	if err != nil {
		return &config, err
	}
	defer configFile.Close()
	decoder := yaml.NewDecoder(configFile)
	err = decoder.Decode(&config)
	return &config, err
}
