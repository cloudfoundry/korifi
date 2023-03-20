package manifest

import (
	"context"
	"errors"
	"fmt"
	"path"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type StateCollector struct {
	appRepo             shared.CFAppRepository
	domainRepo          shared.CFDomainRepository
	processRepo         shared.CFProcessRepository
	routeRepo           shared.CFRouteRepository
	serviceInstanceRepo shared.CFServiceInstanceRepository
	serviceBindingRepo  shared.CFServiceBindingRepository
}

type AppState struct {
	App             repositories.AppRecord
	Processes       map[string]repositories.ProcessRecord
	Routes          map[string]repositories.RouteRecord
	ServiceBindings map[string]repositories.ServiceBindingRecord
}

func NewStateCollector(
	appRepo shared.CFAppRepository,
	domainRepo shared.CFDomainRepository,
	processRepo shared.CFProcessRepository,
	routeRepo shared.CFRouteRepository,
	serviceInstanceRepo shared.CFServiceInstanceRepository,
	serviceBindingRepo shared.CFServiceBindingRepository,
) StateCollector {
	return StateCollector{
		appRepo:             appRepo,
		domainRepo:          domainRepo,
		processRepo:         processRepo,
		routeRepo:           routeRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
	}
}

func (s StateCollector) CollectState(ctx context.Context, authInfo authorization.Info, appName, spaceGUID string) (AppState, error) {
	appRecord, err := s.appRepo.GetAppByNameAndSpace(ctx, authInfo, appName, spaceGUID)
	if err != nil && !errors.As(err, new(apierrors.NotFoundError)) {
		return AppState{}, apierrors.ForbiddenAsNotFound(err)
	}

	existingProcesses := map[string]repositories.ProcessRecord{}
	existingAppRoutes := map[string]repositories.RouteRecord{}
	existingServiceBindings := map[string]repositories.ServiceBindingRecord{}
	if appRecord.GUID != "" {
		procs, err := s.processRepo.ListProcesses(ctx, authInfo, repositories.ListProcessesMessage{
			AppGUIDs:  []string{appRecord.GUID},
			SpaceGUID: spaceGUID,
		})
		if err != nil {
			return AppState{}, err
		}

		for _, p := range procs {
			existingProcesses[p.Type] = p
		}

		routes, err := s.routeRepo.ListRoutesForApp(ctx, authInfo, appRecord.GUID, spaceGUID)
		if err != nil {
			return AppState{}, err
		}
		for _, r := range routes {
			existingAppRoutes[unsplitRoute(r)] = r
		}

		bindings, err := s.serviceBindingRepo.ListServiceBindings(ctx, authInfo, repositories.ListServiceBindingsMessage{
			Type:     "app",
			AppGUIDs: []string{appRecord.GUID},
		})
		if err != nil {
			return AppState{}, err
		}
		for _, b := range bindings {
			instance, err := s.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, b.ServiceInstanceGUID)
			if err != nil {
				return AppState{}, err
			}
			existingServiceBindings[instance.Name] = b
		}
	}

	return AppState{
		App:             appRecord,
		Processes:       existingProcesses,
		Routes:          existingAppRoutes,
		ServiceBindings: existingServiceBindings,
	}, nil
}

func unsplitRoute(route repositories.RouteRecord) string {
	return path.Join(fmt.Sprintf("%s.%s", route.Host, route.Domain.Name), route.Path)
}
