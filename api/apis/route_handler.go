package apis

import (
	"context"
	"errors"
	"net/http"
	"net/url"

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
	RouteAddDestinationsEndpoint = "/v3/routes/{guid}/destinations"
)

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	GetRoute(context.Context, authorization.Info, string) (repositories.RouteRecord, error)
	ListRoutes(context.Context, authorization.Info, repositories.ListRoutesMessage) ([]repositories.RouteRecord, error)
	ListRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	CreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsToRouteMessage) (repositories.RouteRecord, error)
}

type RouteHandler struct {
	logger     logr.Logger
	serverURL  url.URL
	routeRepo  CFRouteRepository
	domainRepo CFDomainRepository
	appRepo    CFAppRepository
}

func NewRouteHandler(
	logger logr.Logger,
	serverURL url.URL,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	appRepo CFAppRepository,
) *RouteHandler {
	return &RouteHandler{
		logger:     logger,
		serverURL:  serverURL,
		routeRepo:  routeRepo,
		domainRepo: domainRepo,
		appRepo:    appRepo,
	}
}

func (h *RouteHandler) routeGetHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Route not found", "RouteGUID", routeGUID)
			writeNotFoundErrorResponse(w, "Route")
			return
		default:
			h.logger.Error(err, "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	err = writeJsonResponse(w, presenter.ForRoute(route, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Route Host", route.Host)
		writeUnknownErrorResponse(w)
	}
}

func (h *RouteHandler) routeGetListHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
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
					writeUnknownKeyError(w, routeListFilter.SupportedFilterKeys())
					return
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return

		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return
		}
	}

	routes, err := h.lookupRouteAndDomainList(ctx, authInfo, routeListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForRouteList(routes, h.serverURL, *r.URL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
}

func (h *RouteHandler) routeGetDestinationsHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Route not found", "RouteGUID", routeGUID)
			writeNotFoundErrorResponse(w, "Route")
			return
		default:
			h.logger.Error(err, "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	err = writeJsonResponse(w, presenter.ForRouteDestinations(route, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Route Host", route.Host)
		writeUnknownErrorResponse(w)
	}
}

func (h *RouteHandler) routeCreateHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.RouteCreate
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	namespaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.appRepo.GetNamespace(ctx, authInfo, namespaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.PermissionDeniedOrNotFoundError:
			h.logger.Info("Namespace not found", "Namespace GUID", namespaceGUID)
			writeUnprocessableEntityError(w, "Invalid space. Ensure that the space exists and you have access to it.")
			return
		default:
			h.logger.Error(err, "Failed to fetch namespace from Kubernetes", "Namespace GUID", namespaceGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	domainGUID := payload.Relationships.Domain.Data.GUID
	domain, err := h.domainRepo.GetDomain(ctx, authInfo, domainGUID)
	if err != nil {
		switch err.(type) {
		case repositories.PermissionDeniedOrNotFoundError:
			h.logger.Info("Domain not found", "Domain GUID", domainGUID)
			writeUnprocessableEntityError(w, "Invalid domain. Ensure that the domain exists and you have access to it.")
			return
		default:
			h.logger.Error(err, "Failed to fetch domain from Kubernetes", "Domain GUID", domainGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	createRouteMessage := payload.ToMessage()

	responseRouteRecord, err := h.routeRepo.CreateRoute(ctx, authInfo, createRouteMessage)
	if err != nil {
		// TODO: Catch the error from the (unwritten) validating webhook
		h.logger.Error(err, "Failed to create route", "Route Host", payload.Host)
		writeUnknownErrorResponse(w)
		return
	}

	responseRouteRecord = responseRouteRecord.UpdateDomainRef(domain)

	err = writeJsonResponse(w, presenter.ForRoute(responseRouteRecord, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Route Host", payload.Host)
		writeUnknownErrorResponse(w)
	}
}

func (h *RouteHandler) routeAddDestinationsHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var destinationCreatePayload payloads.DestinationListCreate
	rme := decodeAndValidateJSONPayload(r, &destinationCreatePayload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	routeRecord, err := h.lookupRouteAndDomain(ctx, routeGUID, authInfo)
	if err != nil {
		if errors.As(err, new(repositories.NotFoundError)) {
			h.logger.Info("Route not found", "RouteGUID", routeGUID)
			writeUnprocessableEntityError(w, "Route is invalid. Ensure it exists and you have access to it.")
		} else {
			h.logger.Error(err, "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	destinationListCreateMessage := destinationCreatePayload.ToMessage(routeRecord)

	responseRouteRecord, err := h.routeRepo.AddDestinationsToRoute(ctx, authInfo, destinationListCreateMessage)
	if err != nil {
		h.logger.Error(err, "Failed to add destination on route", "Route GUID", routeRecord.GUID)
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForRouteDestinations(responseRouteRecord, h.serverURL), http.StatusOK)
	if err != nil { // untested
		h.logger.Error(err, "Failed to render response", "Route GUID", routeRecord.GUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *RouteHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(RouteGetEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.routeGetHandler))
	router.Path(RouteGetListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.routeGetListHandler))
	router.Path(RouteGetDestinationsEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.routeGetDestinationsHandler))
	router.Path(RouteCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.routeCreateHandler))
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
		err = errors.New("resource not found for route's specified domain ref")
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
				err = errors.New("resource not found for route's specified domain ref")
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
				err = errors.New("resource not found for route's specified domain ref")
				return []repositories.RouteRecord{}, err
			}
			domainGUIDToDomainRecord[currentDomainGUID] = domainRecord
		}
		routeRecords[i] = routeRecord.UpdateDomainRef(domainRecord)
	}

	return routeRecords, nil
}
