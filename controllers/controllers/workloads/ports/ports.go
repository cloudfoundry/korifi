package ports

import (
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

func FromRoutes(cfRoutes []korifiv1alpha1.CFRoute, appGUID, processType string) []int32 {
	// In case there are multiple routes, prefer the oldest one
	slices.SortStableFunc(cfRoutes, func(r1, r2 korifiv1alpha1.CFRoute) int {
		return r1.CreationTimestamp.Time.Compare(r2.CreationTimestamp.Time)
	})
	ports := []int32{}
	for _, cfRoute := range cfRoutes {
		for _, destination := range cfRoute.Status.Destinations {
			if destination.AppRef.Name == appGUID &&
				destination.ProcessType == processType &&
				destination.Port != nil {
				ports = append(ports, int32(*destination.Port))
			}
		}
	}

	return ports
}
