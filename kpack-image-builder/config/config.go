package config

import (
	"fmt"
	"os"
)

type ControllerConfig struct {
	CFRootNamespace    string `yaml:"cfRootNamespace"`
	KpackImageTag      string `yaml:"kpackImageTag"`
	ClusterBuilderName string `yaml:"clusterBuilderName"`
}

func LoadFromEnv() *ControllerConfig {
	return &ControllerConfig{
		CFRootNamespace:    mustHaveEnv("ROOT_NAMESPACE"),
		KpackImageTag:      mustHaveEnv("KPACK_IMAGE_TAG"),
		ClusterBuilderName: mustHaveEnv("CLUSTER_BUILDER_NAME"),
	}
}

func mustHaveEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		panic(fmt.Sprintf("Env var %s not set", name))
	}

	return value
}
