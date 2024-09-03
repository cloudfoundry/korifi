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
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/go-logr/logr"
)

const (
	ServiceOfferingsPath = "/v3/service_offerings"
)

//counterfeiter:generate -o fake -fake-name CFServiceOfferingRepository . CFServiceOfferingRepository
type CFServiceOfferingRepository interface {
	ListOfferings(context.Context, authorization.Info, repositories.ListServiceOfferingMessage) ([]repositories.ServiceOfferingRecord, error)
}

type ServiceOffering struct {
	serverURL           url.URL
	requestValidator    RequestValidator
	serviceOfferingRepo CFServiceOfferingRepository
	serviceBrokerRepo   CFServiceBrokerRepository
}

func NewServiceOffering(
	serverURL url.URL,
	requestValidator RequestValidator,
	serviceOfferingRepo CFServiceOfferingRepository,
	serviceBrokerRepo CFServiceBrokerRepository,
) *ServiceOffering {
	return &ServiceOffering{
		serverURL:           serverURL,
		requestValidator:    requestValidator,
		serviceOfferingRepo: serviceOfferingRepo,
		serviceBrokerRepo:   serviceBrokerRepo,
	}
}

func (h *ServiceOffering) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-offering.list")

	var payload payloads.ServiceOfferingList
	if err := h.requestValidator.DecodeAndValidateURLValues(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	serviceOfferingList, err := h.serviceOfferingRepo.ListOfferings(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list service offerings")
	}

	brokerIncludes, err := h.getBrokerIncludes(r.Context(), authInfo, serviceOfferingList, payload.IncludeBrokerFields, h.serverURL)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get broker includes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceOffering, serviceOfferingList, h.serverURL, *r.URL, brokerIncludes...)), nil
}

func (h *ServiceOffering) listBrokersForOfferings(
	ctx context.Context,
	authInfo authorization.Info,
	serviceOfferings []repositories.ServiceOfferingRecord,
) ([]repositories.ServiceBrokerRecord, error) {
	brokerGUIDs := slices.Collect(it.Map(itx.FromSlice(serviceOfferings), func(o repositories.ServiceOfferingRecord) string {
		return o.ServiceBrokerGUID
	}))

	return h.serviceBrokerRepo.ListServiceBrokers(ctx, authInfo, repositories.ListServiceBrokerMessage{
		GUIDs: tools.Uniq(brokerGUIDs),
	})
}

func (h *ServiceOffering) getBrokerIncludes(
	ctx context.Context,
	authInfo authorization.Info,
	serviceOfferings []repositories.ServiceOfferingRecord,
	brokerFields []string,
	baseURL url.URL,
) ([]model.IncludedResource, error) {
	if len(brokerFields) == 0 {
		return nil, nil
	}

	brokers, err := h.listBrokersForOfferings(ctx, authInfo, serviceOfferings)
	if err != nil {
		return nil, err
	}

	return it.TryCollect(it.MapError(slices.Values(brokers), func(broker repositories.ServiceBrokerRecord) (model.IncludedResource, error) {
		return model.IncludedResource{
			Type:     "service_brokers",
			Resource: presenter.ForServiceBroker(broker, baseURL),
		}.SelectJSONPaths(brokerFields...)
	}))
}

func (h *ServiceOffering) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceOffering) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServiceOfferingsPath, Handler: h.list},
	}
}
