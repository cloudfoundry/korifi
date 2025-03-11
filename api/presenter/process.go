package presenter

import (
	"encoding/json"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	processesBase = "/v3/processes"
)

type ProcessResponse struct {
	GUID          string                       `json:"guid"`
	Type          string                       `json:"type"`
	Command       string                       `json:"command"`
	Instances     int32                        `json:"instances"`
	MemoryMB      int64                        `json:"memory_in_mb"`
	DiskQuotaMB   int64                        `json:"disk_in_mb"`
	HealthCheck   ProcessResponseHealthCheck   `json:"health_check"`
	Relationships map[string]ToOneRelationship `json:"relationships"`
	Metadata      Metadata                     `json:"metadata"`
	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
	Links         ProcessLinks                 `json:"links"`
}

type ProcessLinks struct {
	Self  Link `json:"self"`
	Scale Link `json:"scale"`
	App   Link `json:"app"`
	Space Link `json:"space"`
	Stats Link `json:"stats"`
}

type ProcessResponseHealthCheck struct {
	Type string                         `json:"type"`
	Data ProcessResponseHealthCheckData `json:"data"`
}

type ProcessResponseHealthCheckData struct {
	Type              string `json:"-"`
	Timeout           int32  `json:"timeout"`
	InvocationTimeout int32  `json:"invocation_timeout"`
	HTTPEndpoint      string `json:"endpoint"`
}

// alias to avoid infinite recursive in default case of switch below
type respAlias ProcessResponseHealthCheckData

func (h ProcessResponseHealthCheckData) MarshalJSON() ([]byte, error) {
	timeout := tools.PtrTo(h.Timeout)
	if *timeout == 0 {
		timeout = nil
	}
	invocationTimeout := tools.PtrTo(h.InvocationTimeout)
	if *invocationTimeout == 0 {
		invocationTimeout = nil
	}

	switch h.Type {
	case "http":
		return json.Marshal(ProcessResponseHTTPHealthCheckData{
			Timeout:           timeout,
			InvocationTimeout: invocationTimeout,
			HTTPEndpoint:      h.HTTPEndpoint,
		})
	case "port":
		return json.Marshal(ProcessResponsePortHealthCheckData{
			Timeout:           timeout,
			InvocationTimeout: invocationTimeout,
		})
	case "process":
		return json.Marshal(ProcessResponseProcessHealthCheckData{
			Timeout: timeout,
		})
	default:
		return json.Marshal(respAlias(h))
	}
}

type ProcessResponseHTTPHealthCheckData struct {
	Timeout           *int32 `json:"timeout"`
	InvocationTimeout *int32 `json:"invocation_timeout"`
	HTTPEndpoint      string `json:"endpoint"`
}

type ProcessResponsePortHealthCheckData struct {
	Timeout           *int32 `json:"timeout"`
	InvocationTimeout *int32 `json:"invocation_timeout"`
}

type ProcessResponseProcessHealthCheckData struct {
	Timeout *int32 `json:"timeout"`
}

func ForProcess(responseProcess repositories.ProcessRecord, baseURL url.URL) ProcessResponse {
	return ProcessResponse{
		GUID:        responseProcess.GUID,
		Type:        responseProcess.Type,
		Command:     responseProcess.Command,
		Instances:   responseProcess.DesiredInstances,
		MemoryMB:    responseProcess.MemoryMB,
		DiskQuotaMB: responseProcess.DiskQuotaMB,
		HealthCheck: ProcessResponseHealthCheck{
			Type: string(responseProcess.HealthCheck.Type),
			Data: ProcessResponseHealthCheckData{
				Type:              string(responseProcess.HealthCheck.Type),
				Timeout:           responseProcess.HealthCheck.Data.TimeoutSeconds,
				InvocationTimeout: responseProcess.HealthCheck.Data.InvocationTimeoutSeconds,
				HTTPEndpoint:      responseProcess.HealthCheck.Data.HTTPEndpoint,
			},
		},
		Relationships: ForRelationships(responseProcess.Relationships()),
		Metadata: Metadata{
			Labels:      responseProcess.Labels,
			Annotations: responseProcess.Annotations,
		},
		CreatedAt: tools.ZeroIfNil(formatTimestamp(&responseProcess.CreatedAt)),
		UpdatedAt: tools.ZeroIfNil(formatTimestamp(responseProcess.UpdatedAt)),
		Links: ProcessLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(processesBase, responseProcess.GUID).build(),
			},
			Scale: Link{
				HRef:   buildURL(baseURL).appendPath(processesBase, responseProcess.GUID, "actions", "scale").build(),
				Method: http.MethodPost,
			},
			App: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseProcess.AppGUID).build(),
			},
			Space: Link{
				HRef: buildURL(baseURL).appendPath(spacesBase, responseProcess.SpaceGUID).build(),
			},
			Stats: Link{
				HRef: buildURL(baseURL).appendPath(processesBase, responseProcess.GUID, "stats").build(),
			},
		},
	}
}

func ForProcessList(processRecordList []repositories.ProcessRecord, baseURL, requestURL url.URL, includes ...include.Resource) ListResponse[ProcessResponse] {
	return ForList(func(process repositories.ProcessRecord, baseURL url.URL, includes ...include.Resource) ProcessResponse {
		processResponse := ForProcess(process, baseURL)
		processResponse.Command = "[PRIVATE DATA HIDDEN IN LISTS]"
		return processResponse
	}, processRecordList, baseURL, requestURL)
}
