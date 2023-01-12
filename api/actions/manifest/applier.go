package manifest

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

type Applier struct {
	appRepo     shared.CFAppRepository
	domainRepo  shared.CFDomainRepository
	processRepo shared.CFProcessRepository
	routeRepo   shared.CFRouteRepository
}

func NewApplier(
	appRepo shared.CFAppRepository,
	domainRepo shared.CFDomainRepository,
	processRepo shared.CFProcessRepository,
	routeRepo shared.CFRouteRepository,
) *Applier {
	return &Applier{
		appRepo:     appRepo,
		domainRepo:  domainRepo,
		processRepo: processRepo,
		routeRepo:   routeRepo,
	}
}

func (a *Applier) Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication, appState AppState) error {
	appState, err := a.applyApp(ctx, authInfo, spaceGUID, appInfo, appState)
	if err != nil {
		return err
	}

	if err := a.applyProcesses(ctx, authInfo, appInfo, appState); err != nil {
		return err
	}

	return a.applyRoutes(ctx, authInfo, appInfo, appState)
}

func (a *Applier) applyApp(
	ctx context.Context,
	authInfo authorization.Info,
	spaceGUID string,
	appInfo payloads.ManifestApplication,
	appState AppState,
) (AppState, error) {
	if appState.App.GUID == "" {
		appRecord, err := a.appRepo.CreateApp(ctx, authInfo, appInfo.ToAppCreateMessage(spaceGUID))
		return AppState{App: appRecord}, err
	} else {
		_, err := a.appRepo.PatchApp(ctx, authInfo, appInfo.ToAppPatchMessage(appState.App.GUID, spaceGUID))
		return appState, err
	}
}

func (a *Applier) applyProcesses(
	ctx context.Context,
	authInfo authorization.Info,
	appInfo payloads.ManifestApplication,
	appState AppState,
) error {
	for _, processInfo := range appInfo.Processes {
		if process, ok := appState.Processes[processInfo.Type]; ok {
			if _, err := a.processRepo.PatchProcess(ctx, authInfo, processInfo.ToProcessPatchMessage(process.GUID, appState.App.SpaceGUID)); err != nil {
				return err
			}
			continue
		}

		if err := a.processRepo.CreateProcess(ctx, authInfo, processInfo.ToProcessCreateMessage(appState.App.GUID, appState.App.SpaceGUID)); err != nil {
			return err
		}

	}

	return nil
}

func (a *Applier) applyRoutes(ctx context.Context, authInfo authorization.Info, appInfo payloads.ManifestApplication, appState AppState) error {
	if appInfo.NoRoute {
		return a.deleteAppDestinations(ctx, authInfo, appState.App.GUID, appState.Routes)
	}

	return a.createOrUpdateRoutes(ctx, authInfo, appInfo, appState)
}

func (a *Applier) createOrUpdateRoutes(ctx context.Context, authInfo authorization.Info, appInfo payloads.ManifestApplication, appState AppState) error {
	for _, route := range appInfo.Routes {
		err := a.createOrUpdateRoute(ctx, authInfo, *route.Route, appState)
		if err != nil {
			return fmt.Errorf("createOrUpdateRoutes: %w", err)
		}
	}

	return nil
}

func (a *Applier) createOrUpdateRoute(ctx context.Context, authInfo authorization.Info, routeString string, appState AppState) error {
	if _, routeExists := appState.Routes[routeString]; routeExists {
		return nil
	}

	hostName, domainName, path := splitRoute(routeString)

	domainRecord, err := a.domainRepo.GetDomainByName(ctx, authInfo, domainName)
	if err != nil {
		return fmt.Errorf("getDomainByName: %w", err)
	}

	routeRecord, err := a.routeRepo.GetOrCreateRoute(
		ctx,
		authInfo,
		repositories.CreateRouteMessage{
			Host:            hostName,
			Path:            path,
			SpaceGUID:       appState.App.SpaceGUID,
			DomainGUID:      domainRecord.GUID,
			DomainNamespace: domainRecord.Namespace,
			DomainName:      domainRecord.Name,
		})
	if err != nil {
		return fmt.Errorf("getOrCreateRoute: %w", err)
	}

	_, err = a.routeRepo.AddDestinationsToRoute(ctx, authInfo, repositories.AddDestinationsToRouteMessage{
		RouteGUID:            routeRecord.GUID,
		SpaceGUID:            routeRecord.SpaceGUID,
		ExistingDestinations: routeRecord.Destinations,
		NewDestinations: []repositories.DestinationMessage{
			{
				AppGUID:     appState.App.GUID,
				ProcessType: korifiv1alpha1.ProcessTypeWeb,
				Port:        8080,
				Protocol:    "http1",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("addDestinationsToRoute: %w", err)
	}

	return nil
}

func (a *Applier) deleteAppDestinations(
	ctx context.Context,
	authInfo authorization.Info,
	appGUID string,
	existingAppRoutes map[string]repositories.RouteRecord,
) error {
	for _, route := range existingAppRoutes {
		existingDestinations := route.Destinations

		for _, destination := range route.Destinations {
			if destination.AppGUID != appGUID {
				continue
			}

			var err error
			existingDestinations, err = a.deleteAppDestination(ctx, authInfo, route, destination.GUID, existingDestinations)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Applier) deleteAppDestination(ctx context.Context, authInfo authorization.Info, route repositories.RouteRecord, destinationGUID string, existingDestinations []repositories.DestinationRecord) ([]repositories.DestinationRecord, error) {
	route, err := a.routeRepo.RemoveDestinationFromRoute(ctx, authInfo, repositories.RemoveDestinationFromRouteMessage{
		RouteGUID:       route.GUID,
		SpaceGUID:       route.SpaceGUID,
		DestinationGuid: destinationGUID,
	})
	if err != nil {
		return nil, err
	}

	return route.Destinations, nil
}

func splitRoute(route string) (string, string, string) {
	parts := strings.SplitN(route, ".", 2)
	hostName := parts[0]
	domainAndPath := parts[1]

	parts = strings.SplitN(domainAndPath, "/", 2)
	domain := parts[0]
	var path string
	if len(parts) > 1 {
		path = "/" + parts[1]
	}
	return hostName, domain, path
}
