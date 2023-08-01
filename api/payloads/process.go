package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type ProcessScale struct {
	Instances *int   `json:"instances"`
	MemoryMB  *int64 `json:"memory_in_mb"`
	DiskMB    *int64 `json:"disk_in_mb"`
}

func (p ProcessScale) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Instances, validation.Min(0).Error("must be 0 or greater")),
		validation.Field(&p.MemoryMB, validation.Min(1).Error("must be greater than 0")),
		validation.Field(&p.DiskMB, validation.Min(1).Error("must be greater than 0")),
	)
}

type ProcessPatch struct {
	Metadata    *MetadataPatch `json:"metadata"`
	Command     *string        `json:"command"`
	HealthCheck *HealthCheck   `json:"health_check"`
}

type HealthCheck struct {
	Type *string `json:"type"`
	Data *Data   `json:"data"`
}

type Data struct {
	Timeout           *int64  `json:"timeout"`
	Endpoint          *string `json:"endpoint"`
	InvocationTimeout *int64  `json:"invocation_timeout"`
}

func (p ProcessScale) ToRecord() repositories.ProcessScaleValues {
	return repositories.ProcessScaleValues{
		Instances: p.Instances,
		MemoryMB:  p.MemoryMB,
		DiskMB:    p.DiskMB,
	}
}

type ProcessList struct {
	AppGUIDs string
}

func (p *ProcessList) ToMessage() repositories.ListProcessesMessage {
	return repositories.ListProcessesMessage{
		AppGUIDs: parse.ArrayParam(p.AppGUIDs),
	}
}

func (p *ProcessList) SupportedKeys() []string {
	return []string{"app_guids", "per_page", "page"}
}

func (p *ProcessList) DecodeFromURLValues(values url.Values) error {
	p.AppGUIDs = values.Get("app_guids")
	return nil
}

func (p ProcessPatch) ToProcessPatchMessage(processGUID, spaceGUID string) repositories.PatchProcessMessage {
	message := repositories.PatchProcessMessage{
		ProcessGUID: processGUID,
		SpaceGUID:   spaceGUID,
		Command:     p.Command,
	}

	if p.HealthCheck != nil {
		message.HealthCheckType = p.HealthCheck.Type

		if p.HealthCheck.Data != nil {
			message.HealthCheckHTTPEndpoint = p.HealthCheck.Data.Endpoint
			message.HealthCheckTimeoutSeconds = p.HealthCheck.Data.Timeout
			message.HealthCheckInvocationTimeoutSeconds = p.HealthCheck.Data.InvocationTimeout
		}
	}

	if p.Metadata != nil {
		message.MetadataPatch = &repositories.MetadataPatch{
			Annotations: p.Metadata.Annotations,
			Labels:      p.Metadata.Labels,
		}
	}

	return message
}
