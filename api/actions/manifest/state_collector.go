package manifest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/tools/singleton"
	"github.com/BooleanCat/go-functional/v2/it"
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
	listResult, err := s.appRepo.ListApps(ctx, authInfo, repositories.ListAppsMessage{
		Names:      []string{appName},
		SpaceGUIDs: []string{spaceGUID},
	})
	if err != nil {
		return AppState{}, apierrors.FromK8sError(err, repositories.AppResourceType)
	}

	appRecord, err := singleton.Get(listResult.Records)
	if err != nil {
		if errors.As(err, new(apierrors.NotFoundError)) {
			return AppState{}, nil
		}
		return AppState{}, err
	}

	processesByType, err := s.indexProcessesByType(ctx, authInfo, appRecord.GUID, spaceGUID)
	if err != nil {
		return AppState{}, err
	}

	routesByURL, err := s.indexRoutesByURL(ctx, authInfo, appRecord.GUID, spaceGUID)
	if err != nil {
		return AppState{}, err
	}

	bindingsByServiceName, err := s.indexBindingsByServiceName(ctx, authInfo, appRecord.GUID)
	if err != nil {
		return AppState{}, err
	}

	return AppState{
		App:             appRecord,
		Processes:       processesByType,
		Routes:          routesByURL,
		ServiceBindings: bindingsByServiceName,
	}, nil
}

func (s StateCollector) indexProcessesByType(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) (map[string]repositories.ProcessRecord, error) {
	procs, err := s.processRepo.ListProcesses(ctx, authInfo, repositories.ListProcessesMessage{
		AppGUIDs:   []string{appGUID},
		SpaceGUIDs: []string{spaceGUID},
	})
	if err != nil {
		return nil, err
	}

	return index(procs.Records, func(p repositories.ProcessRecord) string {
		return p.Type
	}), nil
}

func (s StateCollector) indexRoutesByURL(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) (map[string]repositories.RouteRecord, error) {
	routes, err := s.routeRepo.ListRoutesForApp(ctx, authInfo, appGUID, spaceGUID)
	if err != nil {
		return nil, err
	}

	return index(routes, func(r repositories.RouteRecord) string {
		return path.Join(fmt.Sprintf("%s.%s", r.Host, r.Domain.Name), r.Path)
	}), nil
}

func (s StateCollector) indexBindingsByServiceName(ctx context.Context, authInfo authorization.Info, appGUID string) (map[string]repositories.ServiceBindingRecord, error) {
	appBindings, err := s.serviceBindingRepo.ListServiceBindings(ctx, authInfo, repositories.ListServiceBindingsMessage{
		AppGUIDs: []string{appGUID},
	})
	if err != nil {
		return nil, err
	}

	appServiceGUIDs := slices.Collect(it.FilterUnique(it.Map(slices.Values(appBindings.Records), func(sb repositories.ServiceBindingRecord) string {
		return sb.ServiceInstanceGUID
	})))

	appServices, err := s.serviceInstanceRepo.ListServiceInstances(ctx, authInfo, repositories.ListServiceInstanceMessage{
		GUIDs: appServiceGUIDs,
	})
	if err != nil {
		return nil, err
	}

	appServicesByGUID := index(appServices, func(s repositories.ServiceInstanceRecord) string {
		return s.GUID
	})

	return tryIndex(appBindings.Records, func(sb repositories.ServiceBindingRecord) (string, error) {
		instance, ok := appServicesByGUID[sb.ServiceInstanceGUID]
		if !ok {
			return "", fmt.Errorf("no service instance found with guid %q for service binding %q", sb.ServiceInstanceGUID, sb.GUID)
		}
		return instance.Name, nil
	})
}

func index[T any](records []T, keyFunc func(T) string) map[string]T {
	recordsIter := slices.Values(records)
	return maps.Collect(it.Zip(
		it.Map(recordsIter, keyFunc),
		recordsIter,
	))
}

func tryIndex[T any](records []T, keyFunc func(T) (string, error)) (map[string]T, error) {
	recordsIter := slices.Values(records)
	recordKeys, err := it.TryCollect(it.MapError(recordsIter, keyFunc))
	if err != nil {
		return nil, fmt.Errorf("failed to index records: %w", err)
	}
	return maps.Collect(it.Zip(
		slices.Values(recordKeys),
		recordsIter,
	)), nil
}
