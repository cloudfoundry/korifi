package presenter

import (
	"maps"
	"slices"
	"strconv"

	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
)

func ForAppManifest(appState manifest.AppState) payloads.ManifestApplication {
	manifestApp := payloads.ManifestApplication{
		Name:      appState.App.Name,
		Processes: toManifestProcesses(appState.Processes),
		Routes:    toManifestRoutes(appState.Routes),
		Services:  toManifestServices(appState.ServiceBindings),
	}

	if appState.Droplet != nil && appState.Droplet.Lifecycle.Type == "docker" {
		manifestApp.Docker = map[string]any{
			"image": appState.Droplet.Image,
		}
	}

	return manifestApp
}

func toManifestRoutes(routes map[string]repositories.RouteRecord) []payloads.ManifestRoute {
	return slices.Collect(it.Map(maps.Keys(routes), func(routeName string) payloads.ManifestRoute {
		return payloads.ManifestRoute{Route: &routeName}
	}))
}

func toManifestProcesses(processes map[string]repositories.ProcessRecord) []payloads.ManifestApplicationProcess {
	return slices.Collect(it.Right(it.Map2(maps.All(processes), func(i string, record repositories.ProcessRecord) (string, payloads.ManifestApplicationProcess) {
		return i, payloads.ManifestApplicationProcess{
			Type:                         i,
			Command:                      tools.PtrTo(record.Command),
			DiskQuota:                    tools.PtrTo(strconv.FormatInt(record.DiskQuotaMB, 10)),
			HealthCheckHTTPEndpoint:      tools.PtrTo(record.HealthCheck.Data.HTTPEndpoint),
			HealthCheckInvocationTimeout: tools.PtrTo(record.HealthCheck.Data.InvocationTimeoutSeconds),
			HealthCheckType:              tools.PtrTo(record.HealthCheck.Type),
			Instances:                    tools.PtrTo(record.DesiredInstances),
			Memory:                       tools.PtrTo(strconv.FormatInt(record.MemoryMB, 10)),
			Timeout:                      tools.PtrTo(record.HealthCheck.Data.TimeoutSeconds),
		}
	})))
}

func toManifestServices(serviceBindings map[string]repositories.ServiceBindingRecord) []payloads.ManifestApplicationService {
	return slices.Collect(it.Right(it.Map2(maps.All(serviceBindings), func(i string, record repositories.ServiceBindingRecord) (string, payloads.ManifestApplicationService) {
		return i, payloads.ManifestApplicationService{
			Name:        record.ServiceInstanceGUID,
			BindingName: record.Name,
		}
	})))
}
