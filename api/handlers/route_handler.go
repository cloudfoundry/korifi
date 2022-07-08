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
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
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
}

type RouteHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
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
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("RouteHandler")),
		serverURL:        serverURL,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		appRepo:          appRepo,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *RouteHandler) routeGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRoute(route, h.serverURL)), nil
}

func (h *RouteHandler) routeGetListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	routeListFilter := new(payloads.RouteList)
	err := payloads.Decode(routeListFilter, r.Form)
	if err != nil {
		logger.Error(err, "Unable to decode request query parameters")
		return nil, err
	}

	routes, err := h.lookupRouteAndDomainList(ctx, authInfo, routeListFilter.ToMessage())
	if err != nil {
		logger.Error(err, "Failed to fetch routes from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *RouteHandler) routeGetDestinationsHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(route, h.serverURL)), nil
}

func (h *RouteHandler) routeCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.RouteCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch space from Kubernetes", "spaceGUID", spaceGUID)
		return nil, apierrors.AsUnprocessableEntity(
			err,
			"Invalid space. Ensure that the space exists and you have access to it.",
			apierrors.NotFoundError{},
			apierrors.ForbiddenError{},
		)
	}

	domainGUID := payload.Relationships.Domain.Data.GUID
	domain, err := h.domainRepo.GetDomain(ctx, authInfo, domainGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch domain from Kubernetes", "Domain GUID", domainGUID)
		return nil, apierrors.AsUnprocessableEntity(
			err,
			"Invalid domain. Ensure that the domain exists and you have access to it.",
			apierrors.NotFoundError{},
		)
	}

	createRouteMessage := payload.ToMessage(domain.Namespace, domain.Name)
	responseRouteRecord, err := h.routeRepo.CreateRoute(ctx, authInfo, createRouteMessage)
	if err != nil {
		logger.Error(err, "Failed to create route", "Route Host", payload.Host)
		return nil, err
	}

	responseRouteRecord = responseRouteRecord.UpdateDomainRef(domain)

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForRoute(responseRouteRecord, h.serverURL)), nil
}

func (h *RouteHandler) routeAddDestinationsHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var destinationCreatePayload payloads.DestinationListCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &destinationCreatePayload); err != nil {
		return nil, err
	}

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	routeRecord, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	destinationListCreateMessage := destinationCreatePayload.ToMessage(routeRecord)

	responseRouteRecord, err := h.routeRepo.AddDestinationsToRoute(ctx, authInfo, destinationListCreateMessage)
	if err != nil {
		logger.Error(err, "Failed to add destination on route", "Route GUID", routeRecord.GUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(responseRouteRecord, h.serverURL)), nil
}

func (h *RouteHandler) routeDeleteDestinationHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	routeGUID := vars["guid"]
	destinationGUID := vars["destination_guid"]

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
		logger.Error(err, "Failed to remove destination from route", "Route GUID", routeRecord.GUID, "Destination GUID", destinationGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusNoContent), nil
}

func (h *RouteHandler) routeDeleteHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	routeRecord, err := h.lookupRouteAndDomain(ctx, logger, authInfo, routeGUID)
	if err != nil {
		return nil, err
	}

	err = h.routeRepo.DeleteRoute(ctx, authInfo, repositories.DeleteRouteMessage{
		GUID:      routeRecord.GUID,
		SpaceGUID: routeRecord.SpaceGUID,
	})
	if err != nil {
		logger.Error(err, "Failed to delete route", "routeGUID", routeGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(routeGUID, presenter.RouteDeleteOperation, h.serverURL)), nil
}

func (h *RouteHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RoutePath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.routeGetHandler))
	router.Path(RoutesPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.routeGetListHandler))
	router.Path(RouteDestinationsPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.routeGetDestinationsHandler))
	router.Path(RoutesPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.routeCreateHandler))
	router.Path(RoutePath).Methods("DELETE").HandlerFunc(h.handlerWrapper.Wrap(h.routeDeleteHandler))
	router.Path(RouteDestinationsPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.routeAddDestinationsHandler))
	router.Path(RouteDestinationPath).Methods("DELETE").HandlerFunc(h.handlerWrapper.Wrap(h.routeDeleteDestinationHandler))
}

// Fetch Route and compose related Domain information within
func (h *RouteHandler) lookupRouteAndDomain(ctx context.Context, logger logr.Logger, authInfo authorization.Info, routeGUID string) (repositories.RouteRecord, error) {
	route, err := h.routeRepo.GetRoute(ctx, authInfo, routeGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
		return repositories.RouteRecord{}, apierrors.ForbiddenAsNotFound(err)
	}

	domain, err := h.domainRepo.GetDomain(ctx, authInfo, route.Domain.GUID)
	if err != nil {
		logger.Error(err, "Failed to fetch domain from Kubernetes", "DomainGUID", route.Domain.GUID)
		return repositories.RouteRecord{}, apierrors.ForbiddenAsNotFound(err)
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
				// err = errors.New("resource not found for route's specified domain ref")
				return []repositories.RouteRecord{}, err
			}
			domainGUIDToDomainRecord[currentDomainGUID] = domainRecord
		}
		routeRecords[i] = routeRecord.UpdateDomainRef(domainRecord)
	}

	return routeRecords, nil
}
