package presenter

import (
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ProcessStatsResponse struct {
	Resources []ProcessStatsResource `json:"resources"`
}

type ProcessStatsResource struct {
	Type             string                 `json:"type"`
	Index            int                    `json:"index"`
	State            string                 `json:"state"`
	Usage            ProcessUsage           `json:"usage"`
	Host             *string                `json:"host"`
	InstancePorts    *[]ProcessInstancePort `json:"instance_ports,omitempty"`
	Uptime           *int                   `json:"uptime"`
	MemQuota         *int                   `json:"mem_quota"`
	DiskQuota        *int                   `json:"disk_quota"`
	FDSQuota         *int                   `json:"fds_quota"`
	IsolationSegment *string                `json:"isolation_segment"`
	Details          *ProcessDetails        `json:"details"`
}

type ProcessUsage struct {
	Time *string  `json:"time,omitempty"`
	CPU  *float64 `json:"cpu,omitempty"`
	Mem  *int64   `json:"mem,omitempty"`
	Disk *int64   `json:"disk,omitempty"`
}

type ProcessInstancePort struct {
	External             int `json:"external"`
	Internal             int `json:"internal"`
	ExternalTLSProxyPort int `json:"external_tls_proxy_port"`
	InternalTLSProxyPort int `json:"internal_tls_proxy_port"`
}

type ProcessDetails struct{}

func ForProcessStats(records []repositories.PodStatsRecord) ProcessStatsResponse {
	resources := []ProcessStatsResource{}
	for _, record := range records {
		resources = append(resources, statRecordToResource(record))
	}
	return ProcessStatsResponse{resources}
}

func statRecordToResource(record repositories.PodStatsRecord) ProcessStatsResource {
	var processInstancePorts *[]ProcessInstancePort
	if record.State != "DOWN" {
		processInstancePorts = &[]ProcessInstancePort{}
	}
	return ProcessStatsResource{
		Type:          record.Type,
		Index:         record.Index,
		State:         record.State,
		InstancePorts: processInstancePorts,
		Usage: ProcessUsage{
			Time: record.Usage.Time,
			CPU:  record.Usage.CPU,
			Mem:  record.Usage.Mem,
			Disk: record.Usage.Disk,
		},
	}
}
