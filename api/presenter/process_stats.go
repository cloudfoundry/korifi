package presenter

import "code.cloudfoundry.org/korifi/api/actions"

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
	MemQuota         *int64                 `json:"mem_quota"`
	DiskQuota        *int64                 `json:"disk_quota"`
	FDSQuota         *int                   `json:"fds_quota"`
	IsolationSegment *string                `json:"isolation_segment"`
	Details          string                 `json:"details"`
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

func ForProcessStats(records []actions.PodStatsRecord) ProcessStatsResponse {
	resources := []ProcessStatsResource{}
	for _, record := range records {
		resources = append(resources, statRecordToResource(record))
	}
	return ProcessStatsResponse{resources}
}

func statRecordToResource(record actions.PodStatsRecord) ProcessStatsResource {
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
		MemQuota:  record.MemQuota,
		DiskQuota: record.DiskQuota,
		Details:   record.Details,
	}
}
