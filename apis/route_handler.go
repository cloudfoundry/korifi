package apis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-api/payloads"
	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RouteGetEndpoint             = "/v3/routes/{guid}"
	RouteGetDestinationsEndpoint = "/v3/routes/{guid}/destinations"
	RouteCreateEndpoint          = "/v3/routes"
)

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	FetchRoute(context.Context, client.Client, string) (repositories.RouteRecord, error)
	CreateRoute(context.Context, client.Client, repositories.RouteRecord) (repositories.RouteRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	FetchDomain(context.Context, client.Client, string) (repositories.DomainRecord, error)
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

	route, err := h.lookupRouteAndDomain(ctx, routeGUID)
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

func (h *RouteHandler) routeGetDestinationsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(ctx, routeGUID)
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

// Fetch Route and compose related Domain information within
func (h *RouteHandler) lookupRouteAndDomain(ctx context.Context, routeGUID string) (repositories.RouteRecord, error) {
	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	route, err := h.routeRepo.FetchRoute(ctx, client, routeGUID)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	domain, err := h.domainRepo.FetchDomain(ctx, client, route.DomainRef.GUID)
	// We assume K8s controller will ensure valid data, so the only error case is due to eventually consistency.
	// Return a generic retryable error.
	if err != nil {
		err = errors.New("resource not found for route's specified domain ref")
		return repositories.RouteRecord{}, err
	}

	route = route.UpdateDomainRef(domain)

	return route, nil
}

func (h *RouteHandler) routeCreateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var routeCreateMessage payloads.RouteCreate
	rme := DecodeAndValidatePayload(r, &routeCreateMessage)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
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

	routeGUID := uuid.New().String()

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

func (h *RouteHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RouteGetEndpoint).Methods("GET").HandlerFunc(h.routeGetHandler)
	router.Path(RouteGetDestinationsEndpoint).Methods("GET").HandlerFunc(h.routeGetDestinationsHandler)
	router.Path(RouteCreateEndpoint).Methods("POST").HandlerFunc(h.routeCreateHandler)
}
