package payloads

import (
	"errors"
	"regexp"

	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/jellydator/validation"

	"code.cloudfoundry.org/bytefmt"
)

type Manifest struct {
	Version      int                   `yaml:"version"`
	Applications []ManifestApplication `json:"applications" yaml:"applications"`
}

type ManifestApplication struct {
	Name         string            `json:"name" yaml:"name"`
	Env          map[string]string `yaml:"env"`
	DefaultRoute bool              `json:"default-route" yaml:"default-route"`
	RandomRoute  bool              `yaml:"random-route"`
	NoRoute      bool              `yaml:"no-route"`
	Command      *string           `yaml:"command"`
	Instances    *int              `json:"instances" yaml:"instances"`
	Memory       *string           `json:"memory" yaml:"memory"`
	DiskQuota    *string           `json:"disk_quota" yaml:"disk_quota"`
	// AltDiskQuota supports `disk-quota` with a hyphen for backwards compatibility.
	// Do not set both DiskQuota and AltDiskQuota.
	//
	// Deprecated: Use DiskQuota instead
	AltDiskQuota                 *string                      `json:"disk-quota" yaml:"disk-quota"`
	HealthCheckHTTPEndpoint      *string                      `yaml:"health-check-http-endpoint"`
	HealthCheckInvocationTimeout *int64                       `json:"health-check-invocation-timeout" yaml:"health-check-invocation-timeout"`
	HealthCheckType              *string                      `json:"health-check-type" yaml:"health-check-type"`
	Timeout                      *int64                       `json:"timeout" yaml:"timeout"`
	Processes                    []ManifestApplicationProcess `json:"processes" yaml:"processes"`
	Routes                       []ManifestRoute              `json:"routes" yaml:"routes"`
	Buildpacks                   []string                     `yaml:"buildpacks"`
	// Deprecated: Use Buildpacks instead
	Buildpack *string                      `json:"buildpack" yaml:"buildpack"`
	Metadata  MetadataPatch                `json:"metadata" yaml:"metadata"`
	Services  []ManifestApplicationService `json:"services" yaml:"services"`
	Docker    any                          `json:"docker,omitempty" yaml:"docker,omitempty"`
}

// TODO: Why is kebab-case used everywhere anyway and we have a deprecated field that claims to use
// it for backwards compatibility?
type ManifestApplicationProcess struct {
	Type      string  `json:"type" yaml:"type"`
	Command   *string `yaml:"command"`
	DiskQuota *string `json:"disk_quota" yaml:"disk_quota"`
	// AltDiskQuota supports `disk-quota` with a hyphen for backwards compatibility.
	// Do not set both DiskQuota and AltDiskQuota.
	//
	// Deprecated: Use DiskQuota instead
	AltDiskQuota                 *string `json:"disk-quota" yaml:"disk-quota"`
	HealthCheckHTTPEndpoint      *string `yaml:"health-check-http-endpoint"`
	HealthCheckInvocationTimeout *int64  `json:"health-check-invocation-timeout" yaml:"health-check-invocation-timeout"`
	HealthCheckType              *string `json:"health-check-type" yaml:"health-check-type"`
	Instances                    *int    `json:"instances" yaml:"instances"`
	Memory                       *string `json:"memory" yaml:"memory"`
	Timeout                      *int64  `json:"timeout" yaml:"timeout"`
}

type ManifestApplicationService struct {
	Name        string  `json:"name" yaml:"name"`
	BindingName *string `json:"binding_name" yaml:"binding_name"`
}

type ManifestRoute struct {
	Route *string `json:"route" yaml:"route"`
}

func (a ManifestApplication) ToAppCreateMessage(spaceGUID string) repositories.CreateAppMessage {
	lifecycle := repositories.Lifecycle{
		Type: string(korifiv1alpha1.BuildpackLifecycle),
		Data: repositories.LifecycleData{
			Buildpacks: a.Buildpacks,
		},
	}

	if a.Docker != nil {
		lifecycle = repositories.Lifecycle{
			Type: string(korifiv1alpha1.DockerPackage),
		}
	}

	return repositories.CreateAppMessage{
		Name:                 a.Name,
		SpaceGUID:            spaceGUID,
		Lifecycle:            lifecycle,
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
		Lifecycle: &repositories.LifecyclePatch{
			Data: &repositories.LifecycleDataPatch{
				Buildpacks: &a.Buildpacks,
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

func (m Manifest) Validate() error {
	return validation.ValidateStruct(&m,
		validation.Field(&m.Applications))
}

func (a ManifestApplication) Validate() error {
	return validation.ValidateStruct(&a,
		validation.Field(&a.Name, validation.Required),
		validation.Field(&a.DefaultRoute, validation.When(a.RandomRoute && a.DefaultRoute, validation.Nil.Error("and random-route may not be used together"))),
		validation.Field(&a.DiskQuota, validation.By(validateAmountWithUnit), validation.When(a.AltDiskQuota != nil, validation.Nil.Error("and disk-quota may not be used together"))),
		validation.Field(&a.AltDiskQuota, validation.By(validateAmountWithUnit)),
		validation.Field(&a.Instances, validation.Min(0)),
		validation.Field(&a.HealthCheckInvocationTimeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
		validation.Field(&a.HealthCheckType, validation.In("none", "process", "port", "http")),
		validation.Field(&a.Memory, validation.By(validateAmountWithUnit)),
		validation.Field(&a.Timeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
		validation.Field(&a.Processes),
		validation.Field(&a.Routes),
		validation.Field(&a.Docker, validation.When(len(a.Buildpacks) > 0 || a.Buildpack != nil,
			validation.Nil.Error("must be blank when buildpacks are specified"),
		)),
	)
}

func (p ManifestApplicationProcess) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Type, validation.Required),
		validation.Field(&p.DiskQuota, validation.By(validateAmountWithUnit), validation.When(p.AltDiskQuota != nil, validation.Nil.Error("and disk-quota may not be used together"))),
		validation.Field(&p.AltDiskQuota, validation.By(validateAmountWithUnit)),
		validation.Field(&p.HealthCheckInvocationTimeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
		validation.Field(&p.HealthCheckType, validation.In("none", "process", "port", "http")),
		validation.Field(&p.Instances, validation.Min(0)),
		validation.Field(&p.Memory, validation.By(validateAmountWithUnit)),
		validation.Field(&p.Timeout, validation.Min(1), validation.NilOrNotEmpty.Error("must be no less than 1")),
	)
}

func (m ManifestRoute) Validate() error {
	routeRegex := regexp.MustCompile(
		`^(?:https?://|tcp://)?(?:(?:[\w-]+\.)|(?:[*]\.))+\w+(?:\:\d+)?(?:/.*)*(?:\.\w+)?$`,
	)
	return validation.ValidateStruct(&m,
		validation.Field(&m.Route, validation.Match(routeRegex).Error("is not a valid route")))
}

func (s ManifestApplicationService) Validate() error {
	return validation.ValidateStruct(&s, validation.Field(&s.Name, validation.Required))
}

var unitAmount = regexp.MustCompile(`^\d+(?:B|K|KB|M|MB|G|GB|T|TB)$`)

func validateAmountWithUnit(value any) error {
	v, isNil := validation.Indirect(value)
	if isNil {
		return nil
	}

	if !unitAmount.MatchString(v.(string)) {
		return errors.New("must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB)")
	}

	mbs, err := bytefmt.ToMegabytes(v.(string))
	if err != nil {
		return err
	}

	if mbs <= 0 {
		return errors.New("must be greater than 0MB")
	}

	return nil
}
