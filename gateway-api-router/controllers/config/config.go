package config

import (
	controllersconfig "code.cloudfoundry.org/korifi/controllers/config"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	CFRootNamespace           string                               `yaml:"cfRootNamespace"`
	CFStagingResources        controllersconfig.CFStagingResources `yaml:"cfStagingResources"`
	BuilderName               string                               `yaml:"clusterBuilderName"`
	ServiceAccount            string                               `yaml:"builderServiceAccount"`
	RunnerName                string                               `yaml:"runnerName"`
	NamespaceLabels           map[string]string                    `yaml:"namespaceLabels"`
	ContainerRepositoryPrefix string                               `yaml:"containerRepositoryPrefix"`
	ContainerRegistryType     string                               `yaml:"containerRegistryType"`
	LogLevel                  zapcore.Level
}
