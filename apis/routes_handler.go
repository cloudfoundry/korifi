package apis

import (
	"encoding/json"
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-api/presenters"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CFRouteRepository interface {
	ConfigureClient(*rest.Config) (client.Client, error)
	FetchRoute(client.Client, string) (repositories.RouteRecord, error)
}

type CFDomainRepository interface {
	ConfigureClient(*rest.Config) (client.Client, error)
	FetchDomain(client.Client, string) (repositories.DomainRecord, error)
}

type RouteHandler struct {
	ServerURL  string
	RouteRepo  CFRouteRepository
	DomainRepo CFDomainRepository
	Logger     logr.Logger
	K8sConfig  *rest.Config // TODO: this would be global for all requests, not what we want
}

func (h *RouteHandler) RouteGetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	routeGUID := vars["guid"]

	route, err := h.lookupRouteAndDomain(routeGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.Logger.Info("Route not found", "RouteGUID", routeGUID)
			writeNotFoundErrorResponse(w, "Route")
			return
		default:
			h.Logger.Error(err, "Failed to fetch route from Kubernetes", "RouteGUID", routeGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenters.NewPresentedRoute(route, h.ServerURL))
	if err != nil {
		panic(err)
	}

	_, _ = w.Write(responseBody)
}

// Fetch Route and compose related Domain information within
func (h *RouteHandler) lookupRouteAndDomain(routeGUID string) (repositories.RouteRecord, error) {
	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	routeClient, err := h.RouteRepo.ConfigureClient(h.K8sConfig)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	route, err := h.RouteRepo.FetchRoute(routeClient, routeGUID)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	domainClient, err := h.DomainRepo.ConfigureClient(h.K8sConfig)
	if err != nil {
		return repositories.RouteRecord{}, err
	}

	domain, err := h.DomainRepo.FetchDomain(domainClient, route.DomainRef.GUID)
	// We assume K8s controller will ensure valid data, so the only error case is due to eventually consistency.
	// Return a generic retryable error.
	if err != nil {
		err = errors.New("resource not found for route's specified domain ref")
		return repositories.RouteRecord{}, err
	}

	route = route.UpdateDomainRef(domain)

	return route, nil
}
