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
	"golang.org/x/exp/maps"
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
	if err != nil {
		if errors.As(err, new(apierrors.NotFoundError)) {
			return AppState{}, nil
		}
		return AppState{}, apierrors.ForbiddenAsNotFound(err)
	}

	existingProcesses, err := s.collectProcesses(ctx, authInfo, appRecord.GUID, spaceGUID)
	if err != nil {
		return AppState{}, err
	}

	existingAppRoutes, err := s.collectRoutes(ctx, authInfo, appRecord.GUID, spaceGUID)
	if err != nil {
		return AppState{}, err
	}

	existingServiceBindings, err := s.collectServiceBindings(ctx, authInfo, appRecord.GUID)
	if err != nil {
		return AppState{}, err
	}

	return AppState{
		App:             appRecord,
		Processes:       existingProcesses,
		Routes:          existingAppRoutes,
		ServiceBindings: existingServiceBindings,
	}, nil
}

func (s StateCollector) collectProcesses(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) (map[string]repositories.ProcessRecord, error) {
	existingProcesses := map[string]repositories.ProcessRecord{}
	procs, err := s.processRepo.ListProcesses(ctx, authInfo, repositories.ListProcessesMessage{
		AppGUIDs:  []string{appGUID},
		SpaceGUID: spaceGUID,
	})
	if err != nil {
		return nil, err
	}

	for _, p := range procs {
		existingProcesses[p.Type] = p
	}

	return existingProcesses, nil
}

func (s StateCollector) collectRoutes(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) (map[string]repositories.RouteRecord, error) {
	existingAppRoutes := map[string]repositories.RouteRecord{}
	routes, err := s.routeRepo.ListRoutesForApp(ctx, authInfo, appGUID, spaceGUID)
	if err != nil {
		return nil, err
	}
	for _, r := range routes {
		existingAppRoutes[unsplitRoute(r)] = r
	}

	return existingAppRoutes, nil
}

func (s StateCollector) collectServiceBindings(ctx context.Context, authInfo authorization.Info, appGUID string) (map[string]repositories.ServiceBindingRecord, error) {
	serviceBindings, err := s.serviceBindingRepo.ListServiceBindings(ctx, authInfo, repositories.ListServiceBindingsMessage{
		AppGUIDs: []string{appGUID},
	})
	if err != nil {
		return nil, err
	}

	serviceInstanceGUIDSet := map[string]bool{}
	for _, sb := range serviceBindings {
		serviceInstanceGUIDSet[sb.ServiceInstanceGUID] = true
	}

	services, err := s.serviceInstanceRepo.ListServiceInstances(ctx, authInfo, repositories.ListServiceInstanceMessage{GUIDs: maps.Keys(serviceInstanceGUIDSet)})
	if err != nil {
		return nil, err
	}

	serviceInstanceGUID2Name := map[string]string{}
	for _, s := range services {
		serviceInstanceGUID2Name[s.GUID] = s.Name
	}

	existingServiceBindings := map[string]repositories.ServiceBindingRecord{}
	for _, sb := range serviceBindings {
		n, ok := serviceInstanceGUID2Name[sb.ServiceInstanceGUID]
		if !ok {
			return nil, fmt.Errorf("no service instance found with guid %q for service binding %q", sb.ServiceInstanceGUID, sb.GUID)
		}
		existingServiceBindings[n] = sb
	}

	return existingServiceBindings, nil
}

func unsplitRoute(route repositories.RouteRecord) string {
	return path.Join(fmt.Sprintf("%s.%s", route.Host, route.Domain.Name), route.Path)
}
