package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type ProcessScale struct {
	Instances *int32 `json:"instances"`
	MemoryMB  *int64 `json:"memory_in_mb"`
	DiskMB    *int64 `json:"disk_in_mb"`
}

func (p ProcessScale) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Instances, jellidation.Min(0).Error("must be 0 or greater")),
		jellidation.Field(&p.MemoryMB, jellidation.Min(1).Error("must be greater than 0")),
		jellidation.Field(&p.DiskMB, jellidation.Min(1).Error("must be greater than 0")),
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
	Timeout           *int32  `json:"timeout"`
	Endpoint          *string `json:"endpoint"`
	InvocationTimeout *int32  `json:"invocation_timeout"`
}

func (p ProcessScale) ToRecord() repositories.ProcessScaleValues {
	return repositories.ProcessScaleValues{
		Instances: p.Instances,
		MemoryMB:  p.MemoryMB,
		DiskMB:    p.DiskMB,
	}
}

type ProcessList struct {
	AppGUIDs   string
	OrderBy    string
	Pagination Pagination
}

func (p ProcessList) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&p.Pagination),
	)
}

func (p *ProcessList) ToMessage() repositories.ListProcessesMessage {
	return repositories.ListProcessesMessage{
		AppGUIDs:   parse.ArrayParam(p.AppGUIDs),
		OrderBy:    p.OrderBy,
		Pagination: p.Pagination.ToMessage(DefaultPageSize),
	}
}

func (p *ProcessList) SupportedKeys() []string {
	return []string{"app_guids", "order_by", "per_page", "page"}
}

func (p *ProcessList) DecodeFromURLValues(values url.Values) error {
	p.AppGUIDs = values.Get("app_guids")
	p.OrderBy = values.Get("order_by")
	return p.Pagination.DecodeFromURLValues(values)
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
