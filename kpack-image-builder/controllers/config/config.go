package config

import (
	"time"

	controllersconfig "code.cloudfoundry.org/korifi/controllers/config"
)

type Config struct {
	CFRootNamespace           string                               `yaml:"cfRootNamespace"`
	CFStagingResources        controllersconfig.CFStagingResources `yaml:"cfStagingResources"`
	ClusterBuilderName        string                               `yaml:"clusterBuilderName"`
	BuilderServiceAccount     string                               `yaml:"builderServiceAccount"`
	BuilderReadinessTimeout   time.Duration                        `yaml:"builderReadinessTimeout"`
	ContainerRepositoryPrefix string                               `yaml:"containerRepositoryPrefix"`
	ContainerRegistryType     string                               `yaml:"containerRegistryType"`
}
