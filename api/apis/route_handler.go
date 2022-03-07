package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
)

const (
	RouteGetEndpoint             = "/v3/routes/{guid}"
	RouteGetListEndpoint         = "/v3/routes"
	RouteGetDestinationsEndpoint = "/v3/routes/{guid}/destinations"
	RouteCreateEndpoint          = "/v3/routes"
	RouteDeleteEndpoint          = "/v3/routes/{guid}"
	RouteAddDestinationsEndpoint = "/v3/routes/{guid}/destinations"
)

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	GetRoute(context.Context, authorization.Info, string) (repositories.RouteRecord, error)
	ListRoutes(context.Context, authorization.Info, repositories.ListRoutesMessage) ([]repositories.RouteRecord, error)
	ListRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	CreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	DeleteRoute(context.Context, authorization.Info, repositories.DeleteRouteMessage) error
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsToRouteMessage) (repositories.RouteRecord, error)
}

type RouteHandler struct {
	logger           logr.Logger
	serverURL        url.URL
	routeRepo        CFRouteRepository
	domainRepo       CFDomainRepository
	appRepo          CFAppRepository
	spaceRepo        SpaceRepository
	decoderValidator *DecoderValidator
}

func NewRouteHandler(
	logger logr.Logger,
	serverURL url.URL,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	appRepo CFAppRepository,
	spaceRepo SpaceRepository,
	decoderValidator *DecoderValidator,
) *RouteHandler {
	return &RouteHandler{
		logger:           logger,
		serverURL:        serverURL,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		appRepo:          appRepo,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *RouteHandler) routeGetHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		return nil, h.catchLookupError(err, routeGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRoute(route, h.serverURL)), nil
}

func (h *RouteHandler) catchLookupError(err error, routeGUID string) error {
	switch typedErr := err.(type) {
	case repositories.NotFoundError:
		h.logger.Info(err.Error(), "RouteGUID", routeGUID)
		return apierrors.NewNotFoundError(err, typedErr.ResourceType())
	case repositories.ForbiddenError:
		h.logger.Info(err.Error(), "RouteGUID", routeGUID)
		return apierrors.NewNotFoundError(err, typedErr.ResourceType())
	case authorization.InvalidAuthError:
		h.logger.Error(err, "unauthorized to get route")
		return apierrors.NewInvalidAuthError(err)
	case authorization.NotAuthenticatedError:
		h.logger.Error(err, "no auth to get route")
		return apierrors.NewNotAuthenticatedError(err)
	default:
		h.logger.Error(err, "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
		return err
	}
}

func (h *RouteHandler) routeGetListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	routeListFilter := new(payloads.RouteList)
	err := schema.NewDecoder().Decode(routeListFilter, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Route filter")
					return nil, apierrors.NewUnknownKeyError(err, routeListFilter.SupportedFilterKeys())
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	routes, err := h.lookupRouteAndDomainList(ctx, authInfo, routeListFilter.ToMessage())
	if err != nil {
		switch err.(type) {
		case authorization.InvalidAuthError:
			h.logger.Error(err, "unauthorized to get routes")
			return nil, apierrors.NewInvalidAuthError(err)
		case authorization.NotAuthenticatedError:
			h.logger.Error(err, "no auth to get routes")
			return nil, apierrors.NewNotAuthenticatedError(err)
		default:
			h.logger.Error(err, "Failed to fetch routes from Kubernetes")
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *RouteHandler) routeGetDestinationsHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		return nil, h.catchLookupError(err, routeGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(route, h.serverURL)), nil
}

func (h *RouteHandler) routeCreateHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	var payload payloads.RouteCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Space not found", "spaceGUID", spaceGUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "Invalid space. Ensure that the space exists and you have access to it.")
		default:
			h.logger.Error(err, "Failed to fetch space from Kubernetes", "spaceGUID", spaceGUID)
			return nil, err
		}
	}

	domainGUID := payload.Relationships.Domain.Data.GUID
	domain, err := h.domainRepo.GetDomain(ctx, authInfo, domainGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Domain not found", "Domain GUID", domainGUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "Invalid domain. Ensure that the domain exists and you have access to it.")
		default:
			h.logger.Error(err, "Failed to fetch domain from Kubernetes", "Domain GUID", domainGUID)
			return nil, err
		}
	}

	createRouteMessage := payload.ToMessage(domain.Namespace)

	responseRouteRecord, err := h.routeRepo.CreateRoute(ctx, authInfo, createRouteMessage)
	if err != nil {
		switch err.(type) {
		case authorization.InvalidAuthError:
			h.logger.Error(err, "unauthorized to create route")
			return nil, apierrors.NewInvalidAuthError(err)
		case authorization.NotAuthenticatedError:
			h.logger.Error(err, "no auth to create route")
			return nil, apierrors.NewNotAuthenticatedError(err)
		case repositories.DuplicateError:
			pathDetails := ""
			if createRouteMessage.Path != "" {
				pathDetails = fmt.Sprintf(" and path '%s'", createRouteMessage.Path)
			}
			errorDetail := fmt.Sprintf("Route already exists with host '%s'%s for domain '%s'.",
				createRouteMessage.Host, pathDetails, domain.Name)
			h.logger.Info(errorDetail)
			return nil, apierrors.NewUnprocessableEntityError(err, errorDetail)
		default:
			h.logger.Error(err, "Failed to create route", "Route Host", payload.Host)
			return nil, err
		}
	}

	responseRouteRecord = responseRouteRecord.UpdateDomainRef(domain)

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForRoute(responseRouteRecord, h.serverURL)), nil
}

func (h *RouteHandler) routeAddDestinationsHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	var destinationCreatePayload payloads.DestinationListCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &destinationCreatePayload); err != nil {
		return nil, err
	}

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	routeRecord, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		return nil, h.catchLookupError(err, routeGUID)
	}

	destinationListCreateMessage := destinationCreatePayload.ToMessage(routeRecord)

	responseRouteRecord, err := h.routeRepo.AddDestinationsToRoute(ctx, authInfo, destinationListCreateMessage)
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Error(err, "not allowed to create route destinations")
			return nil, apierrors.NewForbiddenError(err, repositories.RouteResourceType)
		default:
			h.logger.Error(err, "Failed to add destination on route", "Route GUID", routeRecord.GUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteDestinations(responseRouteRecord, h.serverURL)), nil
}

func (h *RouteHandler) routeDeleteHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	routeRecord, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		return nil, h.catchLookupError(err, routeGUID)
	}

	err = h.routeRepo.DeleteRoute(ctx, authInfo, repositories.DeleteRouteMessage{
		GUID:      routeRecord.GUID,
		SpaceGUID: routeRecord.SpaceGUID,
	})
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Error(err, "unauthorized to delete routes")
			return nil, apierrors.NewForbiddenError(err, repositories.RouteResourceType)
		default:
			h.logger.Error(err, "Failed to delete route", "routeGUID", routeGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", fmt.Sprintf("%s/v3/jobs/route.delete-%s", h.serverURL.String(), routeGUID)), nil
}

func (h *RouteHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(RouteGetEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.routeGetHandler))
	router.Path(RouteGetListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.routeGetListHandler))
	router.Path(RouteGetDestinationsEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.routeGetDestinationsHandler))
	router.Path(RouteCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.routeCreateHandler))
	router.Path(RouteDeleteEndpoint).Methods("DELETE").HandlerFunc(w.Wrap(h.routeDeleteHandler))
	router.Path(RouteAddDestinationsEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.routeAddDestinationsHandler))
}

// Fetch Route and compose related Domain information within
func (h *RouteHandler) lookupRouteAndDomain(ctx context.Context, routeGUID string, authInfo authorization.Info) (repositories.RouteRecord, error) {
	route, err := h.routeRepo.GetRoute(ctx, authInfo, routeGUID)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	domain, err := h.domainRepo.GetDomain(ctx, authInfo, route.Domain.GUID)
	// We assume K8s controller will ensure valid data, so the only error case is due to eventually consistency.
	// Return a generic retryable error.
	if err != nil {
		// err = errors.New("resource not found for route's specified domain ref")
		return repositories.RouteRecord{}, err
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

func getDomainsForRoutes(ctx context.Context, domainRepo CFDomainRepository, authInfo authorization.Info, routeRecords []repositories.RouteRecord) ([]repositories.RouteRecord, error) {
	domainGUIDToDomainRecord := make(map[string]repositories.DomainRecord)
	for i, routeRecord := range routeRecords {
		currentDomainGUID := routeRecord.Domain.GUID
		domainRecord, has := domainGUIDToDomainRecord[currentDomainGUID]
		if !has {
			var err error
			domainRecord, err = domainRepo.GetDomain(ctx, authInfo, currentDomainGUID)
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
