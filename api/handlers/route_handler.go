package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"

	"github.com/go-logr/logr"
)

const (
	RoutePath             = "/v3/routes/{guid}"
	RoutesPath            = "/v3/routes"
	RouteDestinationsPath = "/v3/routes/{guid}/destinations"
	RouteDestinationPath  = "/v3/routes/{guid}/destinations/{destination_guid}"
)

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	GetRoute(context.Context, authorization.Info, string) (repositories.RouteRecord, error)
	ListRoutes(context.Context, authorization.Info, repositories.ListRoutesMessage) ([]repositories.RouteRecord, error)
	ListRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	CreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	DeleteRoute(context.Context, authorization.Info, repositories.DeleteRouteMessage) error
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsToRouteMessage) (repositories.RouteRecord, error)
	RemoveDestinationFromRoute(ctx context.Context, authInfo authorization.Info, message repositories.RemoveDestinationFromRouteMessage) (repositories.RouteRecord, error)
	PatchRouteMetadata(context.Context, authorization.Info, repositories.PatchRouteMetadataMessage) (repositories.RouteRecord, error)
}

type RouteHandler struct {
	serverURL        url.URL
	routeRepo        CFRouteRepository
	domainRepo       CFDomainRepository
	appRepo          CFAppRepository
	spaceRepo        SpaceRepository
	decoderValidator *DecoderValidator
}

func NewRouteHandler(
	serverURL url.URL,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	appRepo CFAppRepository,
	spaceRepo SpaceRepository,
	decoderValidator *DecoderValidator,
) *RouteHandler {
	return &RouteHandler{
		serverURL:        serverURL,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		appRepo:          appRepo,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *RouteHandler) routeGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	routeGUID := URLParam(r, "guid")

	route, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRoute(route, h.serverURL)), nil
}

func (h *RouteHandler) routeGetListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	routeListFilter := new(payloads.RouteList)
	err := payloads.Decode(routeListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	routes, err := h.lookupRouteAndDomainList(ctx, authInfo, routeListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch routes from Kubernetes")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *RouteHandler) routeGetDestinationsHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	routeGUID := URLParam(r, "guid")

	route, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(route, h.serverURL)), nil
}

func (h *RouteHandler) routeCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.RouteCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(
				err,
				"Invalid space. Ensure that the space exists and you have access to it.",
				apierrors.NotFoundError{},
				apierrors.ForbiddenError{},
			),
			"Failed to fetch space from Kubernetes",
			"spaceGUID", spaceGUID,
		)
	}

	domainGUID := payload.Relationships.Domain.Data.GUID
	domain, err := h.domainRepo.GetDomain(ctx, authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger,
			apierrors.AsUnprocessableEntity(
				err,
				"Invalid domain. Ensure that the domain exists and you have access to it.",
				apierrors.NotFoundError{},
			),
			"Failed to fetch space from Kubernetes",
			"spaceGUID", spaceGUID,
		)
	}

	createRouteMessage := payload.ToMessage(domain.Namespace, domain.Name)
	responseRouteRecord, err := h.routeRepo.CreateRoute(ctx, authInfo, createRouteMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create route", "Route Host", payload.Host)
	}

	responseRouteRecord = responseRouteRecord.UpdateDomainRef(domain)

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForRoute(responseRouteRecord, h.serverURL)), nil
}

func (h *RouteHandler) routeAddDestinationsHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var destinationCreatePayload payloads.DestinationListCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &destinationCreatePayload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	routeGUID := URLParam(r, "guid")

	routeRecord, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	destinationListCreateMessage := destinationCreatePayload.ToMessage(routeRecord)

	responseRouteRecord, err := h.routeRepo.AddDestinationsToRoute(ctx, authInfo, destinationListCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to add destination on route", "Route GUID", routeRecord.GUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(responseRouteRecord, h.serverURL)), nil
}

func (h *RouteHandler) routeDeleteDestinationHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	routeGUID := URLParam(r, "guid")
	destinationGUID := URLParam(r, "destination_guid")

	routeRecord, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	message := repositories.RemoveDestinationFromRouteMessage{
		DestinationGuid:      destinationGUID,
		RouteGUID:            routeRecord.GUID,
		SpaceGUID:            routeRecord.SpaceGUID,
		ExistingDestinations: routeRecord.Destinations,
	}

	_, err = h.routeRepo.RemoveDestinationFromRoute(r.Context(), authInfo, message)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to remove destination from route", "Route GUID", routeRecord.GUID, "Destination GUID", destinationGUID)
	}

	return NewHandlerResponse(http.StatusNoContent), nil
}

func (h *RouteHandler) routeDeleteHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	routeGUID := URLParam(r, "guid")

	routeRecord, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	err = h.routeRepo.DeleteRoute(ctx, authInfo, repositories.DeleteRouteMessage{
		GUID:      routeRecord.GUID,
		SpaceGUID: routeRecord.SpaceGUID,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete route", "routeGUID", routeGUID)
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(routeGUID, presenter.RouteDeleteOperation, h.serverURL)), nil
}

func (h *RouteHandler) AuthenticatedRoutes() []Route {
	return []Route{
		{Method: "GET", Pattern: RoutePath, HandlerFunc: h.routeGetHandler},
		{Method: "GET", Pattern: RoutesPath, HandlerFunc: h.routeGetListHandler},
		{Method: "GET", Pattern: RouteDestinationsPath, HandlerFunc: h.routeGetDestinationsHandler},
		{Method: "POST", Pattern: RoutesPath, HandlerFunc: h.routeCreateHandler},
		{Method: "DELETE", Pattern: RoutePath, HandlerFunc: h.routeDeleteHandler},
		{Method: "POST", Pattern: RouteDestinationsPath, HandlerFunc: h.routeAddDestinationsHandler},
		{Method: "DELETE", Pattern: RouteDestinationPath, HandlerFunc: h.routeDeleteDestinationHandler},
		{Method: "PATCH", Pattern: RoutePath, HandlerFunc: h.routePatchHandler},
	}
}

func (h *RouteHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}

// Fetch Route and compose related Domain information within
func (h *RouteHandler) lookupRouteAndDomain(ctx context.Context, logger logr.Logger, authInfo authorization.Info, routeGUID string) (repositories.RouteRecord, error) {
	route, err := h.routeRepo.GetRoute(ctx, authInfo, routeGUID)
	if err != nil {
		return repositories.RouteRecord{}, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
	}

	domain, err := h.domainRepo.GetDomain(ctx, authInfo, route.Domain.GUID)
	if err != nil {
		return repositories.RouteRecord{}, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch domain from Kubernetes", "DomainGUID", route.Domain.GUID)
	}

	route = route.UpdateDomainRef(domain)

	return route, nil
}

func (h *RouteHandler) lookupRouteAndDomainList(ctx context.Context, authInfo authorization.Info, message repositories.ListRoutesMessage) ([]repositories.RouteRecord, error) {
	routeRecords, err := h.routeRepo.ListRoutes(ctx, authInfo, message)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	domainGUIDToDomainRecord := make(map[string]repositories.DomainRecord)

	for i, routeRecord := range routeRecords {
		currentDomainGUID := routeRecord.Domain.GUID
		domainRecord, has := domainGUIDToDomainRecord[currentDomainGUID]
		if !has {
			domainRecord, err = h.domainRepo.GetDomain(ctx, authInfo, currentDomainGUID)
			if err != nil {
				return []repositories.RouteRecord{}, err
			}
			domainGUIDToDomainRecord[currentDomainGUID] = domainRecord
		}
		routeRecords[i] = routeRecord.UpdateDomainRef(domainRecord)
	}

	return routeRecords, nil
}

//nolint:dupl
func (h *RouteHandler) routePatchHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	routeGUID := URLParam(r, "guid")

	route, err := h.routeRepo.GetRoute(ctx, authInfo, routeGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
	}

	var payload payloads.RoutePatch
	if err = h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	route, err = h.routeRepo.PatchRouteMetadata(ctx, authInfo, payload.ToMessage(routeGUID, route.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch route metadata", "RouteGUID", routeGUID)
	}
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRoute(route, h.serverURL)), nil
}
