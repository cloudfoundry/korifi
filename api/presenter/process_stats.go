package presenter

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type ProcessStatsResponse struct {
	Resources []ProcessStatsResource `json:"resources"`
}

type ProcessStatsResource struct {
	Type             string                `json:"type"`
	Index            int                   `json:"index"`
	State            string                `json:"state"`
	Usage            *ProcessUsage         `json:"usage,omitempty"`
	Host             *string               `json:"host"`
	InstancePorts    []ProcessInstancePort `json:"instance_ports"`
	Uptime           *int                  `json:"uptime"`
	MemQuota         *int                  `json:"mem_quota"`
	DiskQuota        *int                  `json:"disk_quota"`
	FDSQuota         *int                  `json:"fds_quota"`
	IsolationSegment *string               `json:"isolation_segment"`
	Details          *ProcessDetails       `json:"details"`
}

type ProcessUsage struct {
	Time *string  `json:"time"`
	CPU  *float64 `json:"cpu"`
	Mem  *int     `json:"mem"`
	Disk *int     `json:"disk"`
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
	return ProcessStatsResource{
		Type:          record.Type,
		Index:         record.Index,
		State:         record.State,
		InstancePorts: []ProcessInstancePort{},
	}
}
