package presenter

import (
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/tools"
)

type ProcessStatsResponse struct {
	Resources []ProcessStatsResource `json:"resources"`
}

type ProcessStatsResource struct {
	Type      string        `json:"type"`
	Index     int           `json:"index"`
	State     string        `json:"state"`
	Usage     *ProcessUsage `json:"usage,omitempty"`
	MemQuota  *int64        `json:"mem_quota,omitempty"`
	DiskQuota *int64        `json:"disk_quota,omitempty"`
}

type ProcessUsage struct {
	Time *string  `json:"time,omitempty"`
	CPU  *float64 `json:"cpu,omitempty"`
	Mem  *int64   `json:"mem,omitempty"`
	Disk *int64   `json:"disk,omitempty"`
}

func ForProcessStats(gauges []stats.ProcessGauges, instancesState []stats.ProcessInstanceState, now time.Time) ProcessStatsResponse {
	gaugesMap := map[int]stats.ProcessGauges{}
	for _, gauge := range gauges {
		gaugesMap[gauge.Index] = gauge
	}

	resources := []ProcessStatsResource{}
	for _, instanceState := range instancesState {
		statsResource := ProcessStatsResource{
			Type:  instanceState.Type,
			Index: instanceState.ID,
			State: string(instanceState.State),
		}

		if gauge, hasGauge := gaugesMap[instanceState.ID]; hasGauge {
			statsResource.Usage = tools.PtrTo(ProcessUsage{
				Time: formatTimestamp(tools.PtrTo(now)),
				CPU:  gauge.CPU,
				Mem:  gauge.Mem,
				Disk: gauge.Disk,
			})
			statsResource.MemQuota = gauge.MemQuota
			statsResource.DiskQuota = gauge.DiskQuota
		}

		resources = append(resources, statsResource)
	}

	slices.SortFunc(resources, func(r1, r2 ProcessStatsResource) int {
		return r1.Index - r2.Index
	})

	return ProcessStatsResponse{resources}
}
