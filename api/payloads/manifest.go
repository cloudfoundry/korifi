package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"code.cloudfoundry.org/bytefmt"
)

type Manifest struct {
	Version      int                   `yaml:"version"`
	Applications []ManifestApplication `yaml:"applications" validate:"max=1,dive"`
}

type ManifestApplication struct {
	Name      string                       `yaml:"name" validate:"required"`
	Env       map[string]string            `yaml:"env"`
	Processes []ManifestApplicationProcess `yaml:"processes" validate:"dive"`
	Routes    []ManifestRoute              `yaml:"routes" validate:"dive"`
}

type ManifestApplicationProcess struct {
	Type                         string  `yaml:"type" validate:"required"`
	Command                      *string `yaml:"command"`
	DiskQuota                    *string `yaml:"disk_quota" validate:"megabytestring"`
	HealthCheckHTTPEndpoint      *string `yaml:"health-check-http-endpoint"`
	HealthCheckInvocationTimeout *int64  `yaml:"health-check-invocation-timeout"`
	HealthCheckType              *string `yaml:"health-check-type"`
	Instances                    *int    `yaml:"instances" validate:"omitempty,gte=0"`
	Memory                       *string `yaml:"memory" validate:"megabytestring"`
	Timeout                      *int64  `yaml:"timeout"`
}

type ManifestRoute struct {
	Route *string `yaml:"route" validate:"route"`
}

func (a ManifestApplication) ToAppCreateMessage(spaceGUID string) repositories.AppCreateMessage {
	return repositories.AppCreateMessage{
		Name:      a.Name,
		SpaceGUID: spaceGUID,
		Lifecycle: repositories.Lifecycle{
			Type: string(v1alpha1.BuildpackLifecycle),
		},
		State:                repositories.DesiredState(v1alpha1.StoppedState),
		EnvironmentVariables: a.Env,
	}
}

func (p ManifestApplicationProcess) ToProcessCreateMessage(appGUID, spaceGUID string) repositories.ProcessCreateMessage {
	var (
		command                      string
		healthCheckType              string
		healthCheckHTTPEndpoint      string
		instances                    int
		healthCheckTimeout           int64
		healthCheckInvocationTimeout int64
		diskQuotaMB                  uint64
		memoryQuotaMB                uint64
	)

	if p.Type == "web" {
		instances = 1
		healthCheckType = "port"
	} else {
		instances = 0
		healthCheckType = "process"
	}

	if p.Command != nil {
		command = *p.Command
	}
	if p.HealthCheckHTTPEndpoint != nil {
		healthCheckHTTPEndpoint = *p.HealthCheckHTTPEndpoint
	}
	if p.HealthCheckInvocationTimeout != nil {
		healthCheckInvocationTimeout = *p.HealthCheckInvocationTimeout
	}
	if p.Timeout != nil {
		healthCheckTimeout = *p.Timeout
	}
	if p.HealthCheckType != nil {
		healthCheckType = *p.HealthCheckType
	}
	if p.Instances != nil {
		instances = *p.Instances
	}

	diskQuotaMB = uint64(1024)
	if p.DiskQuota != nil {
		diskQuotaMB, _ = bytefmt.ToMegabytes(*p.DiskQuota)
	}

	memoryQuotaMB = uint64(1024)
	if p.Memory != nil {
		memoryQuotaMB, _ = bytefmt.ToMegabytes(*p.Memory)
	}

	return repositories.ProcessCreateMessage{
		AppGUID:     appGUID,
		SpaceGUID:   spaceGUID,
		Type:        p.Type,
		Command:     command,
		DiskQuotaMB: int64(diskQuotaMB),
		Healthcheck: repositories.HealthCheck{
			Type: healthCheckType,
			Data: repositories.HealthCheckData{
				HTTPEndpoint:             healthCheckHTTPEndpoint,
				InvocationTimeoutSeconds: healthCheckInvocationTimeout,
				TimeoutSeconds:           healthCheckTimeout,
			},
		},
		DesiredInstances: instances,
		MemoryMB:         int64(memoryQuotaMB),
	}
}

func (p ManifestApplicationProcess) ToProcessPatchMessage(processGUID, spaceGUID string) repositories.ProcessPatchMessage {
	message := repositories.ProcessPatchMessage{
		ProcessGUID:                         processGUID,
		SpaceGUID:                           spaceGUID,
		Command:                             p.Command,
		HealthcheckType:                     p.HealthCheckType,
		HealthCheckHTTPEndpoint:             p.HealthCheckHTTPEndpoint,
		HealthCheckInvocationTimeoutSeconds: p.HealthCheckInvocationTimeout,
		HealthCheckTimeoutSeconds:           p.Timeout,
		DesiredInstances:                    p.Instances,
	}
	if p.DiskQuota != nil {
		diskQuotaMB, _ := bytefmt.ToMegabytes(*p.DiskQuota)
		int64DQMB := int64(diskQuotaMB)
		message.DiskQuotaMB = &int64DQMB
	}
	if p.Memory != nil {
		memoryMB, _ := bytefmt.ToMegabytes(*p.Memory)
		int64MMB := int64(memoryMB)
		message.MemoryMB = &int64MMB
	}
	return message
}
