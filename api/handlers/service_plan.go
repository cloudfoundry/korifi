// nolint:dupl
package handlers

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/iter"
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
	serverURL           url.URL
	requestValidator    RequestValidator
	servicePlanRepo     CFServicePlanRepository
	serviceOfferingRepo CFServiceOfferingRepository
}

func NewServicePlan(
	serverURL url.URL,
	requestValidator RequestValidator,
	servicePlanRepo CFServicePlanRepository,
	serviceOfferingRepo CFServiceOfferingRepository,
) *ServicePlan {
	return &ServicePlan{
		serverURL:           serverURL,
		requestValidator:    requestValidator,
		servicePlanRepo:     servicePlanRepo,
		serviceOfferingRepo: serviceOfferingRepo,
	}
}

func (h *ServicePlan) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-plan.list")

	var payload payloads.ServicePlanList
	if err := h.requestValidator.DecodeAndValidateURLValues(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	servicePlanList, err := h.servicePlanRepo.ListPlans(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list service plans")
	}

	includedResources := []model.IncludedResource{}

	if slices.Contains(payload.IncludeResources, "service_offering") {
		includedOfferings, err := h.getIncludedServiceOfferings(r.Context(), authInfo, servicePlanList, h.serverURL)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "failed to get included service offerings")
		}
		includedResources = append(includedResources, includedOfferings...)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServicePlan, servicePlanList, h.serverURL, *r.URL, includedResources...)), nil
}

func (h *ServicePlan) getIncludedServiceOfferings(
	ctx context.Context,
	authInfo authorization.Info,
	servicePlans []repositories.ServicePlanRecord,
	baseUrl url.URL,
) ([]model.IncludedResource, error) {
	offeringGUIDs := iter.Map(iter.Lift(servicePlans), func(o repositories.ServicePlanRecord) string {
		return o.ServiceOfferingGUID
	}).Collect()

	offerings, err := h.serviceOfferingRepo.ListOfferings(ctx, authInfo, repositories.ListServiceOfferingMessage{
		GUIDs: tools.Uniq(offeringGUIDs),
	})
	if err != nil {
		return nil, err
	}

	return iter.Map(iter.Lift(offerings), func(o repositories.ServiceOfferingRecord) model.IncludedResource {
		return model.IncludedResource{
			Type:     "service_offerings",
			Resource: presenter.ForServiceOffering(o, baseUrl),
		}
	}).Collect(), nil
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
