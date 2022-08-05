package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/rest"
)

const (
	defaultExternalProtocol = "https"
)

type APIConfig struct {
	InternalPort int `yaml:"internalPort"`

	ExternalFQDN string `yaml:"externalFQDN"`
	ExternalPort int    `yaml:"externalPort"`

	ServerURL string

	RootNamespace                            string                 `yaml:"rootNamespace"`
	PackageRegistryBase                      string                 `yaml:"packageRegistryBase"`
	PackageRegistrySecretName                string                 `yaml:"packageRegistrySecretName"`
	DefaultDomainName                        string                 `yaml:"defaultDomainName"`
	UserCertificateExpirationWarningDuration string                 `yaml:"userCertificateExpirationWarningDuration"`
	DefaultLifecycleConfig                   DefaultLifecycleConfig `yaml:"defaultLifecycleConfig"`

	RoleMappings map[string]Role `yaml:"roleMappings"`

	AuthProxyHost   string `yaml:"authProxyHost"`
	AuthProxyCACert string `yaml:"authProxyCACert"`
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

	items, err := os.ReadDir(path)
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

	err = config.validate()
	if err != nil {
		return nil, err
	}

	config.ServerURL, err = config.composeServerURL()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *APIConfig) validate() error {
	if c.ExternalFQDN == "" {
		return errors.New("ExternalFQDN not specified")
	}

	if c.AuthProxyHost != "" && c.AuthProxyCACert == "" {
		return errors.New("AuthProxyHost requires a value for AuthProxyCACert")
	}

	if c.AuthProxyCACert != "" && c.AuthProxyHost == "" {
		return errors.New("AuthProxyCACert requires a value for AuthProxyHost")
	}

	if c.UserCertificateExpirationWarningDuration != "" {
		if _, err := time.ParseDuration(c.UserCertificateExpirationWarningDuration); err != nil {
			return errors.New(`Invalid duration format for userCertificateExpirationWarningDuration. Use a format like "48h"`)
		}
	}

	return nil
}

func (c *APIConfig) GetUserCertificateDuration() time.Duration {
	if c.UserCertificateExpirationWarningDuration == "" {
		return time.Hour * 24 * 7
	}
	d, _ := time.ParseDuration(c.UserCertificateExpirationWarningDuration)
	return d
}

func (c *APIConfig) composeServerURL() (string, error) {
	toReturn := defaultExternalProtocol + "://" + c.ExternalFQDN

	if c.ExternalPort != 0 {
		toReturn += ":" + fmt.Sprint(c.ExternalPort)
	}

	return toReturn, nil
}

func (c *APIConfig) GenerateK8sClientConfig(k8sClientConfig *rest.Config) *rest.Config {
	if c.AuthProxyHost != "" && c.AuthProxyCACert != "" {
		k8sClientConfig.Host = c.AuthProxyHost
		k8sClientConfig.CAData = []byte(c.AuthProxyCACert)
	}

	return k8sClientConfig
}
