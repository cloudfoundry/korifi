package config

import (
	"time"

	"go.uber.org/zap/zapcore"

	"code.cloudfoundry.org/korifi/tools"
)

type ControllerConfig struct {
	// components
	IncludeKpackImageBuilder bool `yaml:"includeKpackImageBuilder"`
	IncludeJobTaskRunner     bool `yaml:"includeJobTaskRunner"`
	IncludeStatefulsetRunner bool `yaml:"includeStatefulsetRunner"`

	// core controllers
	CFProcessDefaults                CFProcessDefaults  `yaml:"cfProcessDefaults"`
	CFStagingResources               CFStagingResources `yaml:"cfStagingResources"`
	CFRootNamespace                  string             `yaml:"cfRootNamespace"`
	ContainerRegistrySecretNames     []string           `yaml:"containerRegistrySecretNames"`
	TaskTTL                          time.Duration      `yaml:"taskTTL"`
	BuilderName                      string             `yaml:"builderName"`
	RunnerName                       string             `yaml:"runnerName"`
	NamespaceLabels                  map[string]string  `yaml:"namespaceLabels"`
	ExtraVCAPApplicationValues       map[string]any     `yaml:"extraVCAPApplicationValues"`
	MaxRetainedPackagesPerApp        int                `yaml:"maxRetainedPackagesPerApp"`
	MaxRetainedBuildsPerApp          int                `yaml:"maxRetainedBuildsPerApp"`
	LogLevel                         zapcore.Level      `yaml:"logLevel"`
	SpaceFinalizerAppDeletionTimeout *int32             `yaml:"spaceFinalizerAppDeletionTimeout"`

	// job-task-runner
	JobTTL time.Duration `yaml:"jobTTL"`

	// kpack-image-builder
	ClusterBuilderName        string        `yaml:"clusterBuilderName"`
	BuilderServiceAccount     string        `yaml:"builderServiceAccount"`
	BuilderReadinessTimeout   time.Duration `yaml:"builderReadinessTimeout"`
	ContainerRepositoryPrefix string        `yaml:"containerRepositoryPrefix"`
	ContainerRegistryType     string        `yaml:"containerRegistryType"`
	Networking                Networking    `yaml:"networking"`

	ExperimentalManagedServicesEnabled bool `yaml:"experimentalManagedServicesEnabled"`
	TrustInsecureServiceBrokers        bool `yaml:"trustInsecureServiceBrokers"`
	DisableRouteController             bool `yaml:"disableRouteController"`
}

type CFProcessDefaults struct {
	MemoryMB    int64  `yaml:"memoryMB"`
	DiskQuotaMB int64  `yaml:"diskQuotaMB"`
	Timeout     *int32 `yaml:"timeout"`
}

type CFStagingResources struct {
	BuildCacheMB int64 `yaml:"buildCacheMB"`
	DiskMB       int64 `yaml:"diskMB"`
	MemoryMB     int64 `yaml:"memoryMB"`
}

type Networking struct {
	GatewayName      string `yaml:"gatewayName"`
	GatewayNamespace string `yaml:"gatewayNamespace"`
}

const (
	defaultTimeout int32 = 60
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

	if config.SpaceFinalizerAppDeletionTimeout == nil {
		config.SpaceFinalizerAppDeletionTimeout = tools.PtrTo(defaultTimeout)
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
