package handlers

import (
	"context"
	"net/http"
	"net/url"
	"sort"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"code.cloudfoundry.org/korifi/api/presenter"

	"code.cloudfoundry.org/korifi/api/repositories"

	"code.cloudfoundry.org/korifi/api/authorization"

	"github.com/go-logr/logr"
)

const (
	ServiceBrokersPath = "/v3/service_brokers"
	ServiceBrokerPath  = "/v3/service_brokers/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFServiceBrokerRepository . CFServiceBrokerRepository
type CFServiceBrokerRepository interface {
	CreateServiceBroker(context.Context, authorization.Info, korifiv1alpha1.ServiceBroker, korifiv1alpha1.BasicAuthentication) (korifiv1alpha1.ServiceBrokerResource, error)
	PatchServiceBroker(context.Context, authorization.Info, string, korifiv1alpha1.ServiceBrokerPatch, *korifiv1alpha1.BasicAuthentication) (korifiv1alpha1.ServiceBrokerResource, error)
	ListServiceBrokers(context.Context, authorization.Info, repositories.ListServiceBrokerMessage) ([]korifiv1alpha1.ServiceBrokerResource, error)
	GetServiceBroker(context.Context, authorization.Info, string) (korifiv1alpha1.ServiceBrokerResource, error)
	DeleteServiceBroker(context.Context, authorization.Info, string) error
}

type ServiceBroker struct {
	serverURL         url.URL
	serviceBrokerRepo CFServiceBrokerRepository
	spaceRepo         CFSpaceRepository
	requestValidator  RequestValidator
}

func NewServiceBroker(
	serverURL url.URL,
	serviceBrokerRepo CFServiceBrokerRepository,
	spaceRepo CFSpaceRepository,
	requestValidator RequestValidator,
) *ServiceBroker {
	return &ServiceBroker{
		serverURL:         serverURL,
		serviceBrokerRepo: serviceBrokerRepo,
		spaceRepo:         spaceRepo,
		requestValidator:  requestValidator,
	}
}

//nolint:dupl
func (h *ServiceBroker) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.create")

	var payload payloads.ServiceBrokerCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	brokerAuth := payload.Authentication
	serviceBroker := payload.ServiceBroker
	serviceBrokerRecord, err := h.serviceBrokerRepo.CreateServiceBroker(r.Context(), authInfo, serviceBroker, brokerAuth)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create service broker", "Service Broker Name", serviceBrokerRecord.Name)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(serviceBrokerRecord.GUID, presenter.ServiceBrokerCreateOperation, h.serverURL)), nil
}

func (h *ServiceBroker) patch(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.patch")

	var payload payloads.ServiceBrokerPatch
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	guid := routing.URLParam(r, "guid")

	serviceBroker, err := h.serviceBrokerRepo.GetServiceBroker(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service broker")
	}

	brokerAuth := payload.Authentication
	serviceBrokerPatch := payload.ServiceBrokerPatch

	serviceBroker, err = h.serviceBrokerRepo.PatchServiceBroker(r.Context(), authInfo, guid, serviceBrokerPatch, brokerAuth)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to patch service broker")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBroker(serviceBroker, h.serverURL)), nil
}

func (h *ServiceBroker) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.list")

	listFilter := new(payloads.ServiceBrokerList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, listFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceBrokerList, err := h.serviceBrokerRepo.ListServiceBrokers(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service brokers")
	}

	h.sortList(serviceBrokerList, listFilter.OrderBy)

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceBroker, serviceBrokerList, h.serverURL, *r.URL)), nil
}

// nolint:dupl
func (h *ServiceBroker) sortList(sbList []korifiv1alpha1.ServiceBrokerResource, order string) {
	switch order {
	case "":
	case "created_at":
		//sort.Slice(sbList, func(i, j int) bool { return timePtrAfter(&sbList[j].CreatedAt, &sbList[i].CreatedAt) })
		sort.Slice(sbList, func(i, j int) bool { return sbList[j].CreatedAt > sbList[i].CreatedAt })
	case "-created_at":
		sort.Slice(sbList, func(i, j int) bool { return sbList[i].CreatedAt > sbList[j].CreatedAt })
	case "updated_at":
		sort.Slice(sbList, func(i, j int) bool { return *sbList[j].UpdatedAt > *sbList[i].UpdatedAt })
	case "-updated_at":
		sort.Slice(sbList, func(i, j int) bool { return *sbList[i].UpdatedAt > *sbList[j].UpdatedAt })
	case "name":
		sort.Slice(sbList, func(i, j int) bool { return sbList[i].Name < sbList[j].Name })
	case "-name":
		sort.Slice(sbList, func(i, j int) bool { return sbList[i].Name > sbList[j].Name })
	}
}

func (h *ServiceBroker) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.delete")

	guid := routing.URLParam(r, "guid")

	_, err := h.serviceBrokerRepo.GetServiceBroker(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service broker")
	}

	err = h.serviceBrokerRepo.DeleteServiceBroker(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service broker", "guid", guid)
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceBroker) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceBroker) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceBrokersPath, Handler: h.create},
		{Method: "PATCH", Pattern: ServiceBrokerPath, Handler: h.patch},
		{Method: "GET", Pattern: ServiceBrokersPath, Handler: h.list},
		{Method: "DELETE", Pattern: ServiceBrokerPath, Handler: h.delete},
	}
}
