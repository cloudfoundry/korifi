package config

import (
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/tools"

	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/rest"
)

const (
	defaultExternalProtocol           = "https"
	OrgRole                 RoleLevel = "org"
	SpaceRole               RoleLevel = "space"
)

type (
	APIConfig struct {
		InternalPort      int `yaml:"internalPort"`
		IdleTimeout       int `yaml:"idleTimeout"`
		ReadTimeout       int `yaml:"readTimeout"`
		ReadHeaderTimeout int `yaml:"readHeaderTimeout"`
		WriteTimeout      int `yaml:"writeTimeout"`

		ExternalFQDN string `yaml:"externalFQDN"`
		ExternalPort int    `yaml:"externalPort"`

		ServerURL string

		InfoConfig InfoConfig `yaml:"infoConfig"`

		RootNamespace                            string                 `yaml:"rootNamespace"`
		BuilderName                              string                 `yaml:"builderName"`
		RunnerName                               string                 `yaml:"runnerName"`
		ContainerRepositoryPrefix                string                 `yaml:"containerRepositoryPrefix"`
		ContainerRegistryType                    string                 `yaml:"containerRegistryType"`
		PackageRegistrySecretNames               []string               `yaml:"packageRegistrySecretNames"`
		DefaultDomainName                        string                 `yaml:"defaultDomainName"`
		UserCertificateExpirationWarningDuration string                 `yaml:"userCertificateExpirationWarningDuration"`
		DefaultLifecycleConfig                   DefaultLifecycleConfig `yaml:"defaultLifecycleConfig"`

		RoleMappings map[string]Role `yaml:"roleMappings"`

		AuthProxyHost   string        `yaml:"authProxyHost"`
		AuthProxyCACert string        `yaml:"authProxyCACert"`
		LogLevel        zapcore.Level `yaml:"logLevel"`

		Experimental Experimental `yaml:"experimental"`
	}

	Experimental struct {
		ManagedServices  ManagedServices `yaml:"managedServices"`
		UAA              UAA             `yaml:"uaa"`
		ExternalLogCache ExtenalLogCache `yaml:"externalLogCache"`
		K8SClient        K8SClientConfig `yaml:"k8sClient"`
		SecurityGroups   SecurityGroups  `yaml:"securityGroups"`
	}

	ManagedServices struct {
		Enabled bool `yaml:"enabled"`
	}

	UAA struct {
		Enabled bool   `yaml:"enabled"`
		URL     string `yaml:"url"`
	}

	ExtenalLogCache struct {
		Enabled               bool   `yaml:"enabled"`
		URL                   string `yaml:"url"`
		TrustInsecureLogCache bool   `yaml:"trustInsecureLogCache"`
	}

	K8SClientConfig struct {
		QPS   float32 `yaml:"qps"`
		Burst int     `yaml:"burst"`
	}

	SecurityGroups struct {
		Enabled bool `yaml:"enabled"`
	}

	RoleLevel string

	Role struct {
		Name      string    `yaml:"name"`
		Level     RoleLevel `yaml:"level"`
		Propagate bool      `yaml:"propagate"`
	}

	// DefaultLifecycleConfig contains default values of the Lifecycle block of CFApps and Builds created by the Shim
	DefaultLifecycleConfig struct {
		Type            string `yaml:"type"`
		Stack           string `yaml:"stack"`
		StagingMemoryMB int    `yaml:"stagingMemoryMB"`
	}

	InfoConfig struct {
		Description           string                 `yaml:"description"`
		Name                  string                 `yaml:"name"`
		MinCLIVersion         string                 `yaml:"minCLIVersion"`
		RecommendedCLIVersion string                 `yaml:"recommendedCLIVersion"`
		Custom                map[string]interface{} `yaml:"custom"`
		SupportAddress        string                 `yaml:"supportAddress"`
	}
)

func LoadFromPath(path string) (*APIConfig, error) {
	var config APIConfig
	err := tools.LoadConfigInto(&config, path)
	if err != nil {
		return nil, err
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

func GetLogLevelFromPath(path string) (zapcore.Level, error) {
	cfg, err := LoadFromPath(path)
	if err != nil {
		return zapcore.InfoLevel, err
	}

	return cfg.LogLevel, nil
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
			return errors.New(`invalid duration format for userCertificateExpirationWarningDuration. Use a format like "48h"`)
		}
	}

	if c.BuilderName == "" {
		return errors.New("BuilderName must have a value")
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

	k8sClientConfig.QPS = c.Experimental.K8SClient.QPS
	k8sClientConfig.Burst = c.Experimental.K8SClient.Burst

	return k8sClientConfig
}
