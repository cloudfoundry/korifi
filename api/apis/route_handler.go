package apis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	FetchRoute(context.Context, client.Client, string) (repositories.RouteRecord, error)
	FetchRouteList(context.Context, client.Client) ([]repositories.RouteRecord, error)
	FetchRoutesForApp(context.Context, client.Client, string, string) ([]repositories.RouteRecord, error)
	CreateRoute(context.Context, client.Client, repositories.RouteRecord) (repositories.RouteRecord, error)
	AddDestinationsToRoute(ctx context.Context, c client.Client, message repositories.RouteAddDestinationsMessage) (repositories.RouteRecord, error)
}

type RouteHandler struct {
	logger      logr.Logger
	serverURL   url.URL
	routeRepo   CFRouteRepository
	domainRepo  CFDomainRepository
	appRepo     CFAppRepository
	buildClient ClientBuilder
	k8sConfig   *rest.Config // TODO: this would be global for all requests, not what we want
}

func NewRouteHandler(
	logger logr.Logger,
	serverURL url.URL,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	appRepo CFAppRepository,
	buildClient ClientBuilder,
	k8sConfig *rest.Config) *RouteHandler {
	return &RouteHandler{
		logger:      logger,
		serverURL:   serverURL,
		routeRepo:   routeRepo,
		domainRepo:  domainRepo,
		appRepo:     appRepo,
		buildClient: buildClient,
		k8sConfig:   k8sConfig,
	}
}

func (h *RouteHandler) routeGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	client, err := h.buildClient(h.k8sConfig, r.Header.Get(headers.Authorization))
	if err != nil {
		h.logger.Error(err, "failed to create kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	route, err := h.lookupRouteAndDomain(ctx, routeGUID, client)
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

	responseBody, err := json.Marshal(presenter.ForRoute(route, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Route Host", route.Host)
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *RouteHandler) routeGetListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	client, err := h.buildClient(h.k8sConfig, r.Header.Get(headers.Authorization))
	if err != nil {
		h.logger.Error(err, "failed to create kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	routes, err := h.lookupRouteAndDomainList(ctx, client)
	if err != nil {
		h.logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForRouteList(routes, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *RouteHandler) routeGetDestinationsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	client, err := h.buildClient(h.k8sConfig, r.Header.Get(headers.Authorization))
	if err != nil {
		h.logger.Error(err, "failed to create kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	route, err := h.lookupRouteAndDomain(ctx, routeGUID, client)
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

	responseBody, err := json.Marshal(presenter.ForRouteDestinations(route, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Route Host", route.Host)
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *RouteHandler) routeCreateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var routeCreateMessage payloads.RouteCreate
	rme := decodeAndValidateJSONPayload(r, &routeCreateMessage)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	client, err := h.buildClient(h.k8sConfig, r.Header.Get(headers.Authorization))
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	namespaceGUID := routeCreateMessage.Relationships.Space.Data.GUID
	_, err = h.appRepo.FetchNamespace(ctx, client, namespaceGUID)
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

	domainGUID := routeCreateMessage.Relationships.Domain.Data.GUID
	domain, err := h.domainRepo.FetchDomain(ctx, client, domainGUID)
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

	routeGUID := uuid.NewString()

	createRouteRecord := routeCreateMessage.ToRecord()
	createRouteRecord.GUID = routeGUID

	responseRouteRecord, err := h.routeRepo.CreateRoute(ctx, client, createRouteRecord)
	if err != nil {
		// TODO: Catch the error from the (unwritten) validating webhook
		h.logger.Error(err, "Failed to create route", "Route Host", routeCreateMessage.Host)
		writeUnknownErrorResponse(w)
		return
	}

	responseRouteRecord = responseRouteRecord.UpdateDomainRef(domain)

	responseBody, err := json.Marshal(presenter.ForRoute(responseRouteRecord, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Route Host", routeCreateMessage.Host)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *RouteHandler) routeAddDestinationsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var destinationCreatePayload payloads.DestinationListCreate
	rme := decodeAndValidateJSONPayload(r, &destinationCreatePayload)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	client, err := h.buildClient(h.k8sConfig, r.Header.Get(headers.Authorization))
	if err != nil {
		h.logger.Error(err, "failed to create kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	routeRecord, err := h.lookupRouteAndDomain(ctx, routeGUID, client)
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

	responseRouteRecord, err := h.routeRepo.AddDestinationsToRoute(ctx, client, destinationListCreateMessage)
	if err != nil {
		h.logger.Error(err, "Failed to add destination on route", "Route GUID", routeRecord.GUID)
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForRouteDestinations(responseRouteRecord, h.serverURL))
	if err != nil { // untested
		h.logger.Error(err, "Failed to render response", "Route GUID", routeRecord.GUID)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *RouteHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RouteGetEndpoint).Methods("GET").HandlerFunc(h.routeGetHandler)
	router.Path(RouteGetListEndpoint).Methods("GET").HandlerFunc(h.routeGetListHandler)
	router.Path(RouteGetDestinationsEndpoint).Methods("GET").HandlerFunc(h.routeGetDestinationsHandler)
	router.Path(RouteCreateEndpoint).Methods("POST").HandlerFunc(h.routeCreateHandler)
	router.Path(RouteAddDestinationsEndpoint).Methods("POST").HandlerFunc(h.routeAddDestinationsHandler)
}

// Fetch Route and compose related Domain information within
func (h *RouteHandler) lookupRouteAndDomain(ctx context.Context, routeGUID string, client client.Client) (repositories.RouteRecord, error) {
	route, err := h.routeRepo.FetchRoute(ctx, client, routeGUID)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	domain, err := h.domainRepo.FetchDomain(ctx, client, route.Domain.GUID)
	// We assume K8s controller will ensure valid data, so the only error case is due to eventually consistency.
	// Return a generic retryable error.
	if err != nil {
		err = errors.New("resource not found for route's specified domain ref")
		return repositories.RouteRecord{}, err
	}

	route = route.UpdateDomainRef(domain)

	return route, nil
}

func (h *RouteHandler) lookupRouteAndDomainList(ctx context.Context, client client.Client) ([]repositories.RouteRecord, error) {
	routeRecords, err := h.routeRepo.FetchRouteList(ctx, client)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	domainGUIDToDomainRecord := make(map[string]repositories.DomainRecord)

	for i, routeRecord := range routeRecords {
		currentDomainGUID := routeRecord.Domain.GUID
		domainRecord, has := domainGUIDToDomainRecord[currentDomainGUID]
		if !has {
			domainRecord, err = h.domainRepo.FetchDomain(ctx, client, currentDomainGUID)
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

func getDomainsForRoutes(ctx context.Context, domainRepo CFDomainRepository, client client.Client, routeRecords []repositories.RouteRecord) ([]repositories.RouteRecord, error) {
	domainGUIDToDomainRecord := make(map[string]repositories.DomainRecord)
	for i, routeRecord := range routeRecords {
		currentDomainGUID := routeRecord.Domain.GUID
		domainRecord, has := domainGUIDToDomainRecord[currentDomainGUID]
		if !has {
			var err error
			domainRecord, err = domainRepo.FetchDomain(ctx, client, currentDomainGUID)
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
