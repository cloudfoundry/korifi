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
	ServiceOfferingsPath = "/v3/service_offerings"
	ServicePlansPath     = "/v3/service_plans"
)

type ServiceCatalogRepo interface {
	ListServiceOfferings(ctx context.Context, authInfo authorization.Info, message repositories.ListServiceOfferingMessage) ([]repositories.ServiceOfferingRecord, error)
	ListServicePlans(ctx context.Context, authInfo authorization.Info, message repositories.ListServicePlanMessage) ([]repositories.ServicePlanRecord, error)
}

type ServiceCatalog struct {
	serverURL          url.URL
	serviceCatalogRepo ServiceCatalogRepo
}

func NewServiceCatalog(
	serverURL url.URL,
	serviceCatalogRepo ServiceCatalogRepo,
) *ServiceCatalog {
	return &ServiceCatalog{
		serverURL:          serverURL,
		serviceCatalogRepo: serviceCatalogRepo,
	}
}

func (h *ServiceCatalog) listOfferings(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	listFilter := new(payloads.ServiceOfferingList)
	err := payloads.Decode(listFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceOfferingList, err := h.serviceCatalogRepo.ListServiceOfferings(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service instance")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceOfferingList(serviceOfferingList, h.serverURL, *r.URL)), nil
}

func (h *ServiceCatalog) listPlans(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	listFilter := new(payloads.ServicePlanList)
	err := payloads.Decode(listFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	servicePlanList, err := h.serviceCatalogRepo.ListServicePlans(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service instance")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServicePlanList(servicePlanList, h.serverURL, *r.URL)), nil
}

func (h *ServiceCatalog) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceCatalog) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServiceOfferingsPath, Handler: h.listOfferings},
		{Method: "GET", Pattern: ServicePlansPath, Handler: h.listPlans},
	}
}
