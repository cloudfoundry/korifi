package repositories

import (
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/BooleanCat/go-functional/v2/it"
)

func FilterRoutesByAppGUID(cfRoutes []korifiv1alpha1.CFRoute, appGuids []string) []korifiv1alpha1.CFRoute {
	routes := slices.Collect(it.Filter(slices.Values(cfRoutes), func(r korifiv1alpha1.CFRoute) bool {
		appsForRoute := slices.Collect(it.Map(slices.Values(r.Spec.Destinations), func(d korifiv1alpha1.Destination) string {
			return d.AppRef.Name
		}))

		for _, app := range appsForRoute {
			if slices.Contains(appGuids, app) {
				return true
			}
		}

		return false
	}))

	return routes
}
