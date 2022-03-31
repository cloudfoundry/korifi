package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultExternalProtocol = "https"
)

type APIConfig struct {
	InternalPort int `yaml:"internalPort"`

	ExternalFQDN string `yaml:"externalFQDN"`
	ExternalPort int    `yaml:"externalPort"`

	ServerURL string

	RootNamespace             string `yaml:"rootNamespace"`
	PackageRegistryBase       string `yaml:"packageRegistryBase"`
	PackageRegistrySecretName string `yaml:"packageRegistrySecretName"`
	ClusterBuilderName        string `yaml:"clusterBuilderName"`
	DefaultDomainName         string `yaml:"defaultDomainName"`

	DefaultLifecycleConfig DefaultLifecycleConfig `yaml:"defaultLifecycleConfig"`

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

		var configFile *os.File
		configFile, err = os.Open(filepath.Join(path, fileName))
		if err != nil {
			return &config, fmt.Errorf("failed to open file: %w", err)
		}
		defer configFile.Close()

		decoder := yaml.NewDecoder(configFile)
		if err = decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("failed decoding %q: %w", item.Name(), err)
		}
	}

	config.ServerURL, err = config.composeServerURL()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *APIConfig) composeServerURL() (string, error) {
	if c.ExternalFQDN == "" {
		return "", errors.New("ExternalFQDN not specified")
	}

	toReturn := defaultExternalProtocol + "://" + c.ExternalFQDN

	if c.ExternalPort != 0 {
		toReturn += ":" + fmt.Sprint(c.ExternalPort)
	}

	return toReturn, nil
}
