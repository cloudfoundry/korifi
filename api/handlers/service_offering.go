package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-logr/logr"
)

const (
	ServiceOfferingsPath = "/v3/service_offerings"
)

//counterfeiter:generate -o fake -fake-name CFServiceOfferingRepository . CFServiceOfferingRepository
type CFServiceOfferingRepository interface {
	ListOfferings(context.Context, authorization.Info) ([]repositories.ServiceOfferingResource, error)
}

type ServiceOffering struct {
	serverURL           url.URL
	serviceOfferingRepo CFServiceOfferingRepository
}

func NewServiceOffering(
	serverURL url.URL,
	serviceOfferingRepo CFServiceOfferingRepository,
) *ServiceOffering {
	return &ServiceOffering{
		serverURL:           serverURL,
		serviceOfferingRepo: serviceOfferingRepo,
	}
}

func (h *ServiceOffering) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-offering.list")

	serviceOfferingList, err := h.serviceOfferingRepo.ListOfferings(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service offerings")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceOffering, serviceOfferingList, h.serverURL, *r.URL)), nil
}

func (h *ServiceOffering) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceOffering) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServiceOfferingsPath, Handler: h.list},
	}
}
