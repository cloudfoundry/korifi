package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

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

type Route struct {
	serverURL        url.URL
	routeRepo        CFRouteRepository
	domainRepo       CFDomainRepository
	appRepo          CFAppRepository
	spaceRepo        SpaceRepository
	decoderValidator *DecoderValidator
}

func NewRoute(
	serverURL url.URL,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	appRepo CFAppRepository,
	spaceRepo SpaceRepository,
	decoderValidator *DecoderValidator,
) *Route {
	return &Route{
		serverURL:        serverURL,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		appRepo:          appRepo,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *Route) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.get")

	routeGUID := routing.URLParam(r, "guid")

	route, err := h.lookupRouteAndDomain(r.Context(), logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRoute(route, h.serverURL)), nil
}

func (h *Route) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	routeListFilter := new(payloads.RouteList)
	err := payloads.Decode(routeListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	routes, err := h.lookupRouteAndDomainList(r.Context(), authInfo, routeListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch routes from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *Route) listDestinations(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.list-destinations")

	routeGUID := routing.URLParam(r, "guid")

	route, err := h.lookupRouteAndDomain(r.Context(), logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(route, h.serverURL)), nil
}

func (h *Route) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.create")

	var payload payloads.RouteCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
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
	domain, err := h.domainRepo.GetDomain(r.Context(), authInfo, domainGUID)
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
	responseRouteRecord, err := h.routeRepo.CreateRoute(r.Context(), authInfo, createRouteMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create route", "Route Host", payload.Host)
	}

	responseRouteRecord = responseRouteRecord.UpdateDomainRef(domain)

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForRoute(responseRouteRecord, h.serverURL)), nil
}

func (h *Route) insertDestinations(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.insert-destinations")

	var destinationCreatePayload payloads.DestinationListCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &destinationCreatePayload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	routeGUID := routing.URLParam(r, "guid")

	routeRecord, err := h.lookupRouteAndDomain(r.Context(), logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	destinationListCreateMessage := destinationCreatePayload.ToMessage(routeRecord)

	responseRouteRecord, err := h.routeRepo.AddDestinationsToRoute(r.Context(), authInfo, destinationListCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to add destination on route", "Route GUID", routeRecord.GUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(responseRouteRecord, h.serverURL)), nil
}

func (h *Route) deleteDestination(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.delete-destination")

	routeGUID := routing.URLParam(r, "guid")
	destinationGUID := routing.URLParam(r, "destination_guid")

	routeRecord, err := h.lookupRouteAndDomain(r.Context(), logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	message := repositories.RemoveDestinationFromRouteMessage{
		DestinationGuid: destinationGUID,
		RouteGUID:       routeGUID,
		SpaceGUID:       routeRecord.SpaceGUID,
	}

	_, err = h.routeRepo.RemoveDestinationFromRoute(r.Context(), authInfo, message)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to remove destination from route", "Route GUID", routeRecord.GUID, "Destination GUID", destinationGUID)
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *Route) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.delete")

	routeGUID := routing.URLParam(r, "guid")

	routeRecord, err := h.lookupRouteAndDomain(r.Context(), logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	err = h.routeRepo.DeleteRoute(r.Context(), authInfo, repositories.DeleteRouteMessage{
		GUID:      routeRecord.GUID,
		SpaceGUID: routeRecord.SpaceGUID,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete route", "routeGUID", routeGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(routeGUID, presenter.RouteDeleteOperation, h.serverURL)), nil
}

func (h *Route) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Route) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: RoutePath, Handler: h.get},
		{Method: "GET", Pattern: RoutesPath, Handler: h.list},
		{Method: "GET", Pattern: RouteDestinationsPath, Handler: h.listDestinations},
		{Method: "POST", Pattern: RoutesPath, Handler: h.create},
		{Method: "DELETE", Pattern: RoutePath, Handler: h.delete},
		{Method: "POST", Pattern: RouteDestinationsPath, Handler: h.insertDestinations},
		{Method: "DELETE", Pattern: RouteDestinationPath, Handler: h.deleteDestination},
		{Method: "PATCH", Pattern: RoutePath, Handler: h.update},
	}
}

// Fetch Route and compose related Domain information within
func (h *Route) lookupRouteAndDomain(ctx context.Context, logger logr.Logger, authInfo authorization.Info, routeGUID string) (repositories.RouteRecord, error) {
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

func (h *Route) lookupRouteAndDomainList(ctx context.Context, authInfo authorization.Info, message repositories.ListRoutesMessage) ([]repositories.RouteRecord, error) {
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
func (h *Route) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.route.update")

	routeGUID := routing.URLParam(r, "guid")

	route, err := h.routeRepo.GetRoute(r.Context(), authInfo, routeGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
	}

	var payload payloads.RoutePatch
	if err = h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	route, err = h.routeRepo.PatchRouteMetadata(r.Context(), authInfo, payload.ToMessage(routeGUID, route.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch route metadata", "RouteGUID", routeGUID)
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRoute(route, h.serverURL)), nil
}
