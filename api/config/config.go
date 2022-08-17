package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/tools"
	"k8s.io/client-go/rest"
)

const (
	defaultExternalProtocol = "https"
)

type APIConfig struct {
	RoleMappings map[string]Role `yaml:"roleMappings"`

	InternalPort int

	ExternalFQDN string
	ExternalPort int

	ServerURL string

	RootNamespace                            string
	PackageRegistryBase                      string
	PackageRegistrySecretName                string
	DefaultDomainName                        string
	UserCertificateExpirationWarningDuration string
	DefaultLifecycleConfig                   DefaultLifecycleConfig

	AuthProxyHost   string
	AuthProxyCACert string
}

type Role struct {
	Name      string `yaml:"name"`
	Propagate bool   `yaml:"propagate"`
}

// DefaultLifecycleConfig contains default values of the Lifecycle block of CFApps and Builds created by the Shim
type DefaultLifecycleConfig struct {
	Type            string
	Stack           string
	StagingMemoryMB int
	StagingDiskMB   int
}

func LoadFromPath(path string) (*APIConfig, error) {
	var config APIConfig
	err := tools.LoadConfigInto(&config, path)
	if err != nil {
		return nil, err
	}

	config.InternalPort = mustHaveIntEnv("INTERNAL_PORT")
	config.ExternalFQDN = mustHaveEnv("EXTERNAL_FQDN")
	config.RootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	config.PackageRegistryBase = mustHaveEnv("PACKAGE_REGISTRY_BASE")
	config.PackageRegistrySecretName = mustHaveEnv("PACKAGE_REGISTRY_SECRET")
	config.DefaultDomainName = mustHaveEnv("DEFAULT_DOMAIN_NAME")
	config.DefaultLifecycleConfig = DefaultLifecycleConfig{
		Type:            mustHaveEnv("LIFECYCLE_TYPE"),
		Stack:           mustHaveEnv("LIFECYCLE_STACK"),
		StagingMemoryMB: mustHaveIntEnv("LIFECYCLE_STAGING_MEMORY_MB"),
		StagingDiskMB:   mustHaveIntEnv("LIFECYCLE_STAGING_DISK_MB"),
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

func mustHaveEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		panic(fmt.Sprintf("Env var %s not set", name))
	}

	return value
}

func mustHaveIntEnv(name string) int {
	value := mustHaveEnv(name)
	intValue, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}

	return intValue
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
