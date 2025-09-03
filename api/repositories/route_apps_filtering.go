package repositories

import (
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
)

func FiterRoutesByApps(cfRouteList *korifiv1alpha1.CFRouteList, appGuids []string) []korifiv1alpha1.CFRoute {
	routes := slices.Collect(it.Filter(slices.Values(cfRouteList.Items), func(r korifiv1alpha1.CFRoute) bool {
		if len(appGuids) == 0 {
			return true
		}

		appsForRoute := slices.Collect(it.Map(slices.Values(r.Spec.Destinations), func(d korifiv1alpha1.Destination) string {
			return d.AppRef.Name
		}))

		for _, app := range appsForRoute {
			if tools.EmptyOrContains(appGuids, app) {
				return true
			}
		}

		return false
	}))

	return routes
}
