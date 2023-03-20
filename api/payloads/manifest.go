package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	"code.cloudfoundry.org/bytefmt"
)

type Manifest struct {
	Version      int                   `yaml:"version"`
	Applications []ManifestApplication `yaml:"applications" validate:"dive"`
}

type ManifestApplication struct {
	Name         string            `yaml:"name" validate:"required"`
	Env          map[string]string `yaml:"env"`
	DefaultRoute bool              `yaml:"default-route"`
	RandomRoute  bool              `yaml:"random-route"`
	NoRoute      bool              `yaml:"no-route"`
	Command      *string           `yaml:"command"`
	Instances    *int              `yaml:"instances" validate:"omitempty,gte=0"`
	Memory       *string           `yaml:"memory" validate:"amountWithUnit,positiveAmountWithUnit"`
	DiskQuota    *string           `yaml:"disk_quota" validate:"amountWithUnit,positiveAmountWithUnit"`
	// AltDiskQuota supports `disk-quota` with a hyphen for backwards compatibility.
	// Do not set both DiskQuota and AltDiskQuota.
	//
	// Deprecated: Use DiskQuota instead
	AltDiskQuota                 *string                      `yaml:"disk-quota" validate:"amountWithUnit,positiveAmountWithUnit"`
	HealthCheckHTTPEndpoint      *string                      `yaml:"health-check-http-endpoint"`
	HealthCheckInvocationTimeout *int64                       `yaml:"health-check-invocation-timeout" validate:"omitempty,gte=1"`
	HealthCheckType              *string                      `yaml:"health-check-type" validate:"omitempty,oneof=none process port http"`
	Timeout                      *int64                       `yaml:"timeout" validate:"omitempty,gte=1"`
	Processes                    []ManifestApplicationProcess `yaml:"processes" validate:"dive"`
	Routes                       []ManifestRoute              `yaml:"routes" validate:"dive"`
	Buildpacks                   []string                     `yaml:"buildpacks"`
	Services                     []string                     `yaml:"services"`

	// Deprecated: Use Buildpacks instead
	Buildpack string        `yaml:"buildpack"`
	Metadata  MetadataPatch `yaml:"metadata"`
}

type ManifestApplicationProcess struct {
	Type      string  `yaml:"type" validate:"required"`
	Command   *string `yaml:"command"`
	DiskQuota *string `yaml:"disk_quota" validate:"amountWithUnit,positiveAmountWithUnit"`
	// AltDiskQuota supports `disk-quota` with a hyphen for backwards compatibility.
	// Do not set both DiskQuota and AltDiskQuota.
	//
	// Deprecated: Use DiskQuota instead
	AltDiskQuota                 *string `yaml:"disk-quota" validate:"amountWithUnit,positiveAmountWithUnit"`
	HealthCheckHTTPEndpoint      *string `yaml:"health-check-http-endpoint"`
	HealthCheckInvocationTimeout *int64  `yaml:"health-check-invocation-timeout" validate:"omitempty,gte=1"`
	HealthCheckType              *string `yaml:"health-check-type" validate:"omitempty,oneof=none process port http"`
	Instances                    *int    `yaml:"instances" validate:"omitempty,gte=0"`
	Memory                       *string `yaml:"memory" validate:"amountWithUnit,positiveAmountWithUnit"`
	Timeout                      *int64  `yaml:"timeout" validate:"omitempty,gte=1"`
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
		Metadata: repositories.Metadata{
			Labels:      ignoreNilKeys(a.Metadata.Labels),
			Annotations: ignoreNilKeys(a.Metadata.Annotations),
		},
	}
}

func ignoreNilKeys(m map[string]*string) map[string]string {
	result := map[string]string{}
	for k, v := range m {
		if v == nil {
			continue
		}
		result[k] = *v
	}
	return result
}

func (a ManifestApplication) ToAppPatchMessage(appGUID, spaceGUID string) repositories.PatchAppMessage {
	return repositories.PatchAppMessage{
		Name:      a.Name,
		AppGUID:   appGUID,
		SpaceGUID: spaceGUID,
		Lifecycle: repositories.Lifecycle{
			Type: string(korifiv1alpha1.BuildpackLifecycle),
			Data: repositories.LifecycleData{
				Buildpacks: a.Buildpacks,
			},
		},
		EnvironmentVariables: a.Env,
		MetadataPatch:        repositories.MetadataPatch(a.Metadata),
	}
}

func (p ManifestApplicationProcess) ToProcessCreateMessage(appGUID, spaceGUID string) repositories.CreateProcessMessage {
	msg := repositories.CreateProcessMessage{
		AppGUID:   appGUID,
		SpaceGUID: spaceGUID,
		Type:      p.Type,
	}

	if p.Command != nil {
		msg.Command = *p.Command
	}
	if p.HealthCheckHTTPEndpoint != nil {
		msg.HealthCheck.Data.HTTPEndpoint = *p.HealthCheckHTTPEndpoint
	}
	if p.HealthCheckInvocationTimeout != nil {
		msg.HealthCheck.Data.InvocationTimeoutSeconds = *p.HealthCheckInvocationTimeout
	}
	if p.Timeout != nil {
		msg.HealthCheck.Data.TimeoutSeconds = *p.Timeout
	}
	if p.HealthCheckType != nil {
		msg.HealthCheck.Type = *p.HealthCheckType
		if msg.HealthCheck.Type == "none" {
			msg.HealthCheck.Type = "process"
		}
	}
	msg.DesiredInstances = p.Instances

	if p.Memory != nil {
		// error ignored intentionally, since the manifest yaml is validated in handlers
		memoryQuotaMB, _ := bytefmt.ToMegabytes(*p.Memory)
		msg.MemoryMB = int64(memoryQuotaMB)
	}

	if p.DiskQuota != nil {
		// error ignored intentionally, since the manifest yaml is validated in handlers
		diskQuotaMB, _ := bytefmt.ToMegabytes(*p.DiskQuota)
		msg.DiskQuotaMB = int64(diskQuotaMB)
	}

	return msg
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
		message.HealthCheckType = p.HealthCheckType
		if *message.HealthCheckType == "none" {
			message.HealthCheckType = tools.PtrTo("process")
		}
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
