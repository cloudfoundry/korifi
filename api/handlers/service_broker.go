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
	ServiceBrokersPath = "/v3/service_brokers"
	ServiceBrokerPath  = "/v3/service_brokers/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFServiceBrokerRepository . CFServiceBrokerRepository
type CFServiceBrokerRepository interface {
	CreateServiceBroker(context.Context, authorization.Info, repositories.CreateServiceBrokerMessage) (repositories.ServiceBrokerRecord, error)
	ListServiceBrokers(context.Context, authorization.Info, repositories.ListServiceBrokerMessage) ([]repositories.ServiceBrokerRecord, error)
	GetServiceBroker(context.Context, authorization.Info, string) (repositories.ServiceBrokerRecord, error)
	DeleteServiceBroker(context.Context, authorization.Info, string) error
	UpdateServiceBroker(context.Context, authorization.Info, repositories.UpdateServiceBrokerMessage) (repositories.ServiceBrokerRecord, error)
}

type ServiceBroker struct {
	serverURL         url.URL
	serviceBrokerRepo CFServiceBrokerRepository
	requestValidator  RequestValidator
}

func NewServiceBroker(
	serverURL url.URL,
	serviceBrokerRepo CFServiceBrokerRepository,
	requestValidator RequestValidator,
) *ServiceBroker {
	return &ServiceBroker{
		serverURL:         serverURL,
		serviceBrokerRepo: serviceBrokerRepo,
		requestValidator:  requestValidator,
	}
}

func (h *ServiceBroker) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.create")

	payload := payloads.ServiceBrokerCreate{}
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	broker, err := h.serviceBrokerRepo.CreateServiceBroker(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create service broker")
	}

	return routing.NewResponse(http.StatusAccepted).
		WithHeader("Location", presenter.JobURLForRedirects(broker.GUID, presenter.ServiceBrokerCreateOperation, h.serverURL)), nil
}

func (h *ServiceBroker) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.list")

	var serviceBrokerListFilter payloads.ServiceBrokerList
	if err := h.requestValidator.DecodeAndValidateURLValues(r, &serviceBrokerListFilter); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode request values")
	}

	brokers, err := h.serviceBrokerRepo.ListServiceBrokers(r.Context(), authInfo, serviceBrokerListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list service brokers")
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceBroker, brokers, h.serverURL, *r.URL)), nil
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

	return routing.NewResponse(http.StatusAccepted).
		WithHeader("Location", presenter.JobURLForRedirects(guid, presenter.ServiceBrokerDeleteOperation, h.serverURL)), nil
}

func (h *ServiceBroker) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.update")

	guid := routing.URLParam(r, "guid")
	payload := payloads.ServiceBrokerUpdate{}
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	_, err := h.serviceBrokerRepo.GetServiceBroker(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service broker")
	}

	broker, err := h.serviceBrokerRepo.UpdateServiceBroker(r.Context(), authInfo, payload.ToMessage(guid))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service broker", "guid", guid)
	}

	if payload.IsAsyncRequest() {
		return routing.NewResponse(http.StatusAccepted).
			WithHeader("Location", presenter.JobURLForRedirects(broker.GUID, presenter.ServiceBrokerUpdateOperation, h.serverURL)), nil
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBroker(broker, h.serverURL)), nil
}

func (h *ServiceBroker) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceBroker) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceBrokersPath, Handler: h.create},
		{Method: "GET", Pattern: ServiceBrokersPath, Handler: h.list},
		{Method: "DELETE", Pattern: ServiceBrokerPath, Handler: h.delete},
		{Method: "PATCH", Pattern: ServiceBrokerPath, Handler: h.update},
	}
}
