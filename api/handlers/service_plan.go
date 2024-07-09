// nolint:dupl
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
	ServicePlansPath = "/v3/service_plans"
)

//counterfeiter:generate -o fake -fake-name CFServicePlanRepository . CFServicePlanRepository
type CFServicePlanRepository interface {
	ListPlans(context.Context, authorization.Info) ([]repositories.ServicePlanResource, error)
}

type ServicePlan struct {
	serverURL       url.URL
	servicePlanRepo CFServicePlanRepository
}

func NewServicePlan(
	serverURL url.URL,
	servicePlanRepo CFServicePlanRepository,
) *ServicePlan {
	return &ServicePlan{
		serverURL:       serverURL,
		servicePlanRepo: servicePlanRepo,
	}
}

func (h *ServicePlan) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-plan.list")

	servicePlanList, err := h.servicePlanRepo.ListPlans(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service plans")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServicePlan, servicePlanList, h.serverURL, *r.URL)), nil
}

func (h *ServicePlan) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServicePlan) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServicePlansPath, Handler: h.list},
	}
}
