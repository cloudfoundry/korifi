package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"code.cloudfoundry.org/bytefmt"
)

const (
	processTypeWeb         = "web"
	processHealthCheckType = "process"
)

type Manifest struct {
	Version      int                   `yaml:"version"`
	Applications []ManifestApplication `yaml:"applications" validate:"max=1,dive"`
}

type ManifestApplication struct {
	Name         string                       `yaml:"name" validate:"required"`
	Env          map[string]string            `yaml:"env"`
	DefaultRoute bool                         `yaml:"default-route"`
	RandomRoute  bool                         `yaml:"random-route"`
	NoRoute      bool                         `yaml:"no-route"`
	Memory       *string                      `yaml:"memory" validate:"megabytestring"`
	Processes    []ManifestApplicationProcess `yaml:"processes" validate:"dive"`
	Routes       []ManifestRoute              `yaml:"routes" validate:"dive"`
	Buildpacks   []string                     `yaml:"buildpacks"`
}

type ManifestApplicationProcess struct {
	Type                         string  `yaml:"type" validate:"required"`
	Command                      *string `yaml:"command"`
	DiskQuota                    *string `yaml:"disk_quota" validate:"megabytestring"`
	HealthCheckHTTPEndpoint      *string `yaml:"health-check-http-endpoint"`
	HealthCheckInvocationTimeout *int64  `yaml:"health-check-invocation-timeout"`
	HealthCheckType              *string `yaml:"health-check-type" validate:"omitempty,oneof=none process port http"`
	Instances                    *int    `yaml:"instances" validate:"omitempty,gte=0"`
	Memory                       *string `yaml:"memory" validate:"megabytestring"`
	Timeout                      *int64  `yaml:"timeout"`
}

type ManifestRoute struct {
	Route *string `yaml:"route" validate:"route"`
}

func (a ManifestApplication) ToAppCreateMessage(spaceGUID string) repositories.CreateAppMessage {
	return repositories.CreateAppMessage{
		Name:      a.Name,
		SpaceGUID: spaceGUID,
		Lifecycle: repositories.Lifecycle{
			Type: string(korifiv1alpha1.BuildpackLifecycle),
			Data: repositories.LifecycleData{
				Buildpacks: a.Buildpacks,
			},
		},
		State:                repositories.DesiredState(korifiv1alpha1.StoppedState),
		EnvironmentVariables: a.Env,
	}
}

func (p ManifestApplicationProcess) ToProcessCreateMessage(appGUID, spaceGUID string) repositories.CreateProcessMessage {
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

	healthCheckType = processHealthCheckType
	if p.Type == processTypeWeb {
		instances = 1
	} else {
		instances = 0
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
		healthCheckType = normalizeHealthCheckType(*p.HealthCheckType)
	}
	if p.Instances != nil {
		instances = *p.Instances
	}

	diskQuotaMB = uint64(1024)
	if p.DiskQuota != nil {
		// error ignored intentionally, since the manifest yaml is validated in handlers
		diskQuotaMB, _ = bytefmt.ToMegabytes(*p.DiskQuota)
	}

	memoryQuotaMB = uint64(1024)
	if p.Memory != nil {
		// error ignored intentionally, since the manifest yaml is validated in handlers
		memoryQuotaMB, _ = bytefmt.ToMegabytes(*p.Memory)
	}

	return repositories.CreateProcessMessage{
		AppGUID:     appGUID,
		SpaceGUID:   spaceGUID,
		Type:        p.Type,
		Command:     command,
		DiskQuotaMB: int64(diskQuotaMB),
		HealthCheck: repositories.HealthCheck{
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

func (p ManifestApplicationProcess) ToProcessPatchMessage(processGUID, spaceGUID string) repositories.PatchProcessMessage {
	message := repositories.PatchProcessMessage{
		ProcessGUID:                         processGUID,
		SpaceGUID:                           spaceGUID,
		Command:                             p.Command,
		HealthCheckHTTPEndpoint:             p.HealthCheckHTTPEndpoint,
		HealthCheckInvocationTimeoutSeconds: p.HealthCheckInvocationTimeout,
		HealthCheckTimeoutSeconds:           p.Timeout,
		DesiredInstances:                    p.Instances,
	}
	if p.HealthCheckType != nil {
		healthCheckType := normalizeHealthCheckType(*p.HealthCheckType)
		message.HealthCheckType = &healthCheckType
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

func normalizeHealthCheckType(healthCheckType string) string {
	const NoneHealthCheckType = "none"

	switch healthCheckType {
	case NoneHealthCheckType:
		return string(korifiv1alpha1.ProcessHealthCheckType)
	default:
		return healthCheckType
	}
}
