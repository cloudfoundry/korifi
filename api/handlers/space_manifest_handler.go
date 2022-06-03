package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	processTypeWeb = "web"

	SpaceManifestApplyPath = "/v3/spaces/{spaceGUID}/actions/apply_manifest"
	SpaceManifestDiffPath  = "/v3/spaces/{spaceGUID}/manifest_diff"
)

//counterfeiter:generate -o fake -fake-name CFManifestRepository . CFManifestRepository

type CFManifestRepository interface {
	GetSpace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)

	GetAppByNameAndSpace(context.Context, authorization.Info, string, string) (repositories.AppRecord, error)
	CreateOrPatchAppEnvVars(context.Context, authorization.Info, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.CreateAppMessage) (repositories.AppRecord, error)

	GetDomainByName(context.Context, authorization.Info, string) (repositories.DomainRecord, error)

	CreateProcess(context.Context, authorization.Info, repositories.CreateProcessMessage) error
	GetProcessByAppTypeAndSpace(context.Context, authorization.Info, string, string, string) (repositories.ProcessRecord, error)
	PatchProcess(context.Context, authorization.Info, repositories.PatchProcessMessage) (repositories.ProcessRecord, error)

	GetOrCreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	ListRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsToRouteMessage) (repositories.RouteRecord, error)
}

type SpaceManifestHandler struct {
	handlerWrapper    *AuthAwareHandlerFuncWrapper
	serverURL         url.URL
	defaultDomainName string
	decoderValidator  *DecoderValidator
	manifestRepo      CFManifestRepository
}

func NewSpaceManifestHandler(
	serverURL url.URL,
	defaultDomainName string,
	decoderValidator *DecoderValidator,
	manifestRepo CFManifestRepository,
) *SpaceManifestHandler {
	return &SpaceManifestHandler{
		handlerWrapper:    NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("SpaceManifestHandler")),
		serverURL:         serverURL,
		defaultDomainName: defaultDomainName,
		decoderValidator:  decoderValidator,
		manifestRepo:      manifestRepo,
	}
}

func (h *SpaceManifestHandler) RegisterRoutes(router *mux.Router) {
	router.Path(SpaceManifestApplyPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.applyManifestHandler))
	router.Path(SpaceManifestDiffPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.diffManifestHandler))
}

func (h *SpaceManifestHandler) applyManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	spaceGUID := vars["spaceGUID"]
	var manifest payloads.Manifest
	if err := h.decoderValidator.DecodeAndValidateYAMLPayload(r, &manifest); err != nil {
		return nil, err
	}

	if err := h.applyManifest(ctx, authInfo, spaceGUID, h.defaultDomainName, manifest); err != nil {
		logger.Error(err, "Error applying manifest")
		return nil, err
	}

	return NewHandlerResponse(http.StatusAccepted).
		WithHeader(headers.Location, presenter.JobURLForRedirects(spaceGUID, presenter.SpaceApplyManifestOperation, h.serverURL)), nil
}

func (h *SpaceManifestHandler) diffManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	spaceGUID := vars["spaceGUID"]

	if _, err := h.manifestRepo.GetSpace(r.Context(), authInfo, spaceGUID); err != nil {
		logger.Error(err, "failed to get space", "guid", spaceGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusAccepted).WithBody(map[string]interface{}{"diff": []string{}}), nil
}

func (h *SpaceManifestHandler) applyManifest(ctx context.Context, authInfo authorization.Info, spaceGUID string, defaultDomainName string, manifest payloads.Manifest) error {
	appInfo := manifest.Applications[0]
	exists := true
	appRecord, err := h.manifestRepo.GetAppByNameAndSpace(ctx, authInfo, appInfo.Name, spaceGUID)
	if err != nil {
		if errors.As(err, new(apierrors.NotFoundError)) {
			exists = false
		} else {
			return apierrors.ForbiddenAsNotFound(err)
		}
	}

	if appInfo.Memory != nil {
		found := false
		for _, process := range appInfo.Processes {
			if process.Type == processTypeWeb {
				found = true
			}
		}

		if !found {
			appInfo.Processes = append(appInfo.Processes, payloads.ManifestApplicationProcess{
				Type:   processTypeWeb,
				Memory: appInfo.Memory,
			})
		}
	}

	if exists {
		err = h.updateApp(ctx, authInfo, spaceGUID, appRecord, appInfo)
	} else {
		appRecord, err = h.createApp(ctx, authInfo, spaceGUID, appInfo)
	}

	if err != nil {
		return err
	}

	err = h.checkAndUpdateDefaultRoute(ctx, authInfo, appRecord, defaultDomainName, &appInfo)
	if err != nil {
		return err
	}

	return h.createOrUpdateRoutes(ctx, authInfo, appRecord, appInfo.Routes)
}

// checkAndUpdateDefaultRoute may set the default route on the manifest when DefaultRoute is true
func (h *SpaceManifestHandler) checkAndUpdateDefaultRoute(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, defaultDomainName string, appInfo *payloads.ManifestApplication) error {
	if !appInfo.DefaultRoute || len(appInfo.Routes) > 0 {
		return nil
	}

	existingRoutes, err := h.manifestRepo.ListRoutesForApp(ctx, authInfo, appRecord.GUID, appRecord.SpaceGUID)
	if err != nil {
		return err
	}
	if len(existingRoutes) > 0 {
		return nil
	}

	_, err = h.manifestRepo.GetDomainByName(ctx, authInfo, defaultDomainName)
	if err != nil {
		return apierrors.AsUnprocessableEntity(
			err,
			fmt.Sprintf("The configured default domain %q was not found", defaultDomainName),
			apierrors.NotFoundError{},
		)
	}
	defaultRouteString := appInfo.Name + "." + defaultDomainName
	defaultRoute := payloads.ManifestRoute{
		Route: &defaultRouteString,
	}
	// set the route field of the manifest with app-name . default domain
	appInfo.Routes = append(appInfo.Routes, defaultRoute)

	return nil
}

func (h *SpaceManifestHandler) updateApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication) error {
	_, err := h.manifestRepo.CreateOrPatchAppEnvVars(ctx, authInfo, repositories.CreateOrPatchAppEnvVarsMessage{
		AppGUID:              appRecord.GUID,
		AppEtcdUID:           appRecord.EtcdUID,
		SpaceGUID:            appRecord.SpaceGUID,
		EnvironmentVariables: appInfo.Env,
	})
	if err != nil {
		return err
	}

	for _, processInfo := range appInfo.Processes {
		exists := true

		var process repositories.ProcessRecord
		process, err = h.manifestRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, appRecord.GUID, processInfo.Type, spaceGUID)
		if err != nil {
			if errors.As(err, new(apierrors.NotFoundError)) {
				exists = false
			} else {
				return err
			}
		}

		if exists {
			_, err = h.manifestRepo.PatchProcess(ctx, authInfo, processInfo.ToProcessPatchMessage(process.GUID, spaceGUID))
		} else {
			err = h.manifestRepo.CreateProcess(ctx, authInfo, processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID))
		}
		if err != nil {
			return err
		}
	}

	return err
}

func (h *SpaceManifestHandler) createApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication) (repositories.AppRecord, error) {
	appRecord, err := h.manifestRepo.CreateApp(ctx, authInfo, appInfo.ToAppCreateMessage(spaceGUID))
	if err != nil {
		return appRecord, err
	}

	for _, processInfo := range appInfo.Processes {
		message := processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID)
		err = h.manifestRepo.CreateProcess(ctx, authInfo, message)
		if err != nil {
			return appRecord, err
		}
	}

	return appRecord, nil
}

func (h *SpaceManifestHandler) createOrUpdateRoutes(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, routes []payloads.ManifestRoute) error {
	if len(routes) == 0 {
		return nil
	}

	routeString := *routes[0].Route
	hostName, domainName, path := splitRoute(routeString)

	domainRecord, err := h.manifestRepo.GetDomainByName(ctx, authInfo, domainName)
	if err != nil {
		return fmt.Errorf("createOrUpdateRoutes: %w", err)
	}

	routeRecord, err := h.manifestRepo.GetOrCreateRoute(
		ctx,
		authInfo,
		repositories.CreateRouteMessage{
			Host:            hostName,
			Path:            path,
			SpaceGUID:       appRecord.SpaceGUID,
			DomainGUID:      domainRecord.GUID,
			DomainNamespace: domainRecord.Namespace,
			DomainName:      domainRecord.Name,
		})
	if err != nil {
		return fmt.Errorf("createOrUpdateRoutes: %w", err)
	}

	routeRecord, err = h.manifestRepo.AddDestinationsToRoute(ctx, authInfo, repositories.AddDestinationsToRouteMessage{
		RouteGUID:            routeRecord.GUID,
		SpaceGUID:            routeRecord.SpaceGUID,
		ExistingDestinations: routeRecord.Destinations,
		NewDestinations: []repositories.DestinationMessage{
			{
				AppGUID:     appRecord.GUID,
				ProcessType: "web",
				Port:        8080,
				Protocol:    "http1",
			},
		},
	})

	return err
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
