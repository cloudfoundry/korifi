// nolint:dupl
package handlers

import (
	"context"
	"fmt"
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
	serviceBrokerRepo   CFServiceBrokerRepository
}

func NewServicePlan(
	serverURL url.URL,
	requestValidator RequestValidator,
	servicePlanRepo CFServicePlanRepository,
	serviceOfferingRepo CFServiceOfferingRepository,
	serviceBrokerRepo CFServiceBrokerRepository,
) *ServicePlan {
	return &ServicePlan{
		serverURL:           serverURL,
		requestValidator:    requestValidator,
		servicePlanRepo:     servicePlanRepo,
		serviceOfferingRepo: serviceOfferingRepo,
		serviceBrokerRepo:   serviceBrokerRepo,
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

	includedResources, err := h.getIncludedResources(r.Context(), authInfo, payload, servicePlans)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServicePlan, servicePlans, h.serverURL, *r.URL, includedResources...)), nil
}

func (h *ServicePlan) getIncludedResources(
	ctx context.Context,
	authInfo authorization.Info,
	payload payloads.ServicePlanList,
	servicePlans []repositories.ServicePlanRecord,
) ([]model.IncludedResource, error) {
	if len(payload.IncludeResources) == 0 && len(payload.IncludeBrokerFields) == 0 {
		return nil, nil
	}

	includedResources := []model.IncludedResource{}

	offerings, err := h.listOfferings(ctx, authInfo, servicePlans)
	if err != nil {
		return nil, fmt.Errorf("failed to list offerings for plans: %w", err)
	}

	if slices.Contains(payload.IncludeResources, "service_offering") {
		includedResources = append(includedResources, iter.Map(iter.Lift(offerings), func(o repositories.ServiceOfferingRecord) model.IncludedResource {
			return model.IncludedResource{
				Type:     "service_offerings",
				Resource: presenter.ForServiceOffering(o, h.serverURL),
			}
		}).Collect()...)
	}

	if len(payload.IncludeBrokerFields) != 0 {
		brokers, err := h.listBrokers(ctx, authInfo, offerings)
		if err != nil {
			return nil, fmt.Errorf("failed to list brokers for offerings of plans: %w", err)
		}

		includedBrokerFields, err := h.getIncludedBrokerFields(brokers, payload.IncludeBrokerFields)
		if err != nil {
			return nil, fmt.Errorf("failed to get included broker fields: %w", err)
		}
		includedResources = append(includedResources, includedBrokerFields...)
	}

	return includedResources, nil
}

func (h *ServicePlan) listOfferings(
	ctx context.Context,
	authInfo authorization.Info,
	servicePlans []repositories.ServicePlanRecord,
) ([]repositories.ServiceOfferingRecord, error) {
	offeringGUIDs := iter.Map(iter.Lift(servicePlans), func(o repositories.ServicePlanRecord) string {
		return o.ServiceOfferingGUID
	}).Collect()

	return h.serviceOfferingRepo.ListOfferings(ctx, authInfo, repositories.ListServiceOfferingMessage{
		GUIDs: tools.Uniq(offeringGUIDs),
	})
}

func (h *ServicePlan) listBrokers(
	ctx context.Context,
	authInfo authorization.Info,
	offerings []repositories.ServiceOfferingRecord,
) ([]repositories.ServiceBrokerRecord, error) {
	brokerGUIDs := iter.Map(iter.Lift(offerings), func(o repositories.ServiceOfferingRecord) string {
		return o.ServiceBrokerGUID
	}).Collect()

	return h.serviceBrokerRepo.ListServiceBrokers(ctx, authInfo, repositories.ListServiceBrokerMessage{
		GUIDs: brokerGUIDs,
	})
}

func (h *ServicePlan) getIncludedBrokerFields(
	brokers []repositories.ServiceBrokerRecord,
	brokerFields []string,
) ([]model.IncludedResource, error) {
	brokerIncludes := iter.Map(iter.Lift(brokers), func(b repositories.ServiceBrokerRecord) model.IncludedResource {
		return model.IncludedResource{
			Type:     "service_brokers",
			Resource: presenter.ForServiceBroker(b, h.serverURL),
		}
	}).Collect()

	brokerIncludesFields := []model.IncludedResource{}
	for _, brokerInclude := range brokerIncludes {
		fields, err := brokerInclude.SelectJSONFields(brokerFields...)
		if err != nil {
			return nil, err
		}
		brokerIncludesFields = append(brokerIncludesFields, fields)
	}

	return brokerIncludesFields, nil
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
