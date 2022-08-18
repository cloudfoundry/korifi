package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/tools"
)

type ControllerConfig struct {
	CFProcessDefaults           CFProcessDefaults
	CFRootNamespace             string
	PackageRegistrySecretName   string
	TaskTTL                     string
	WorkloadsTLSSecretName      string
	WorkloadsTLSSecretNamespace string
	BuildReconciler             string
	AppReconciler               string
}

type CFProcessDefaults struct {
	MemoryMB    int64 `yaml:"memoryMB"`
	DiskQuotaMB int64 `yaml:"diskQuotaMB"`
}

const defaultTaskTTL = 30 * 24 * time.Hour

func LoadFromEnv() *ControllerConfig {
	return &ControllerConfig{
		CFProcessDefaults: CFProcessDefaults{
			MemoryMB:    mustHaveIntEnv("PROCESS_DEFAULT_MEMORY_MB"),
			DiskQuotaMB: mustHaveIntEnv("PROCESS_DEFAULT_DISK_QUOTA_MB"),
		},
		CFRootNamespace:             mustHaveEnv("ROOT_NAMESPACE"),
		PackageRegistrySecretName:   mustHaveEnv("PACKAGE_REGISTRY_SECRET_NAME"),
		TaskTTL:                     mustHaveEnv("TASK_TTL"),
		WorkloadsTLSSecretName:      mustHaveEnv("WORKLOADS_TLS_SECRET_NAME"),
		WorkloadsTLSSecretNamespace: mustHaveEnv("WORKLOADS_TLS_SECRET_NAMESPACE"),
		BuildReconciler:             mustHaveEnv("BUILD_RECONCILER"),
		AppReconciler:               mustHaveEnv("APP_RECONCILER"),
	}
}

func (c ControllerConfig) WorkloadsTLSSecretNameWithNamespace() string {
	if c.WorkloadsTLSSecretName == "" {
		return ""
	}
	return filepath.Join(c.WorkloadsTLSSecretNamespace, c.WorkloadsTLSSecretName)
}

func (c ControllerConfig) ParseTaskTTL() (time.Duration, error) {
	if c.TaskTTL == "" {
		return defaultTaskTTL, nil
	}

	return tools.ParseDuration(c.TaskTTL)
}

func mustHaveEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		panic(fmt.Sprintf("Env var %s not set", name))
	}

	return value
}

func mustHaveIntEnv(name string) int64 {
	value := mustHaveEnv(name)
	intValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		panic(err)
	}

	return intValue
}
