// nolint:dupl
package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers/include"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-logr/logr"
)

const (
	ServicePlansPath          = "/v3/service_plans"
	ServicePlanVisivilityPath = "/v3/service_plans/{guid}/visibility"
)

//counterfeiter:generate -o fake -fake-name CFServicePlanRepository . CFServicePlanRepository
type CFServicePlanRepository interface {
	GetPlan(context.Context, authorization.Info, string) (repositories.ServicePlanRecord, error)
	ListPlans(context.Context, authorization.Info, repositories.ListServicePlanMessage) ([]repositories.ServicePlanRecord, error)
	ApplyPlanVisibility(context.Context, authorization.Info, repositories.ApplyServicePlanVisibilityMessage) (repositories.ServicePlanRecord, error)
	UpdatePlanVisibility(context.Context, authorization.Info, repositories.UpdateServicePlanVisibilityMessage) (repositories.ServicePlanRecord, error)
}

type ServicePlan struct {
	serverURL        url.URL
	requestValidator RequestValidator
	servicePlanRepo  CFServicePlanRepository
	includeResolver  *include.IncludeResolver[
		[]repositories.ServicePlanRecord,
		repositories.ServicePlanRecord,
	]
}

func NewServicePlan(
	serverURL url.URL,
	requestValidator RequestValidator,
	servicePlanRepo CFServicePlanRepository,
	relationshipRepo include.ResourceRelationshipRepository,
) *ServicePlan {
	return &ServicePlan{
		serverURL:        serverURL,
		requestValidator: requestValidator,
		servicePlanRepo:  servicePlanRepo,
		includeResolver:  include.NewIncludeResolver[[]repositories.ServicePlanRecord](relationshipRepo, presenter.NewResource(serverURL)),
	}
}

func (h *ServicePlan) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-plan.list")

	var payload payloads.ServicePlanList
	if err := h.requestValidator.DecodeAndValidateURLValues(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	servicePlans, err := h.servicePlanRepo.ListPlans(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list service plans")
	}

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, servicePlans, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServicePlan, servicePlans, h.serverURL, *r.URL, includedResources...)), nil
}

func (h *ServicePlan) getPlanVisibility(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-plan.get-visibility")

	planGUID := routing.URLParam(r, "guid")
	logger = logger.WithValues("guid", planGUID)

	plan, err := h.servicePlanRepo.GetPlan(r.Context(), authInfo, planGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get plan visibility")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServicePlanVisibility(plan, h.serverURL)), nil
}

func (h *ServicePlan) applyPlanVisibility(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-plan.apply-visibility")

	planGUID := routing.URLParam(r, "guid")
	logger = logger.WithValues("guid", planGUID)

	payload := payloads.ServicePlanVisibility{}
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	visibility, err := h.servicePlanRepo.ApplyPlanVisibility(r.Context(), authInfo, payload.ToApplyMessage(planGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to apply plan visibility")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServicePlanVisibility(visibility, h.serverURL)), nil
}

func (h *ServicePlan) updatePlanVisibility(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-plan.update-visibility")

	planGUID := routing.URLParam(r, "guid")
	logger = logger.WithValues("guid", planGUID)

	payload := payloads.ServicePlanVisibility{}
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	visibility, err := h.servicePlanRepo.UpdatePlanVisibility(r.Context(), authInfo, payload.ToUpdateMessage(planGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to update plan visibility")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServicePlanVisibility(visibility, h.serverURL)), nil
}

func (h *ServicePlan) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServicePlan) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServicePlansPath, Handler: h.list},
		{Method: "GET", Pattern: ServicePlanVisivilityPath, Handler: h.getPlanVisibility},
		{Method: "POST", Pattern: ServicePlanVisivilityPath, Handler: h.applyPlanVisibility},
		{Method: "PATCH", Pattern: ServicePlanVisivilityPath, Handler: h.updatePlanVisibility},
	}
}
