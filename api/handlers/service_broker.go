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
)

//counterfeiter:generate -o fake -fake-name CFServiceBrokerRepository . CFServiceBrokerRepository
type CFServiceBrokerRepository interface {
	CreateServiceBroker(context.Context, authorization.Info, repositories.CreateServiceBrokerMessage) (repositories.ServiceBrokerResource, error)
	ListServiceBrokers(context.Context, authorization.Info) ([]repositories.ServiceBrokerResource, error)
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

	broker, err := h.serviceBrokerRepo.CreateServiceBroker(r.Context(), authInfo, payload.ToCreateServiceBrokerMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create service broker")
	}

	return routing.NewResponse(http.StatusAccepted).
		WithHeader("Location", presenter.JobURLForRedirects(broker.GUID, presenter.ServiceBrokerCreateOperation, h.serverURL)), nil
}

func (h *ServiceBroker) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-broker.list")

	brokers, err := h.serviceBrokerRepo.ListServiceBrokers(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list service brokers")
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceBroker, brokers, h.serverURL, *r.URL)), nil
}

func (h *ServiceBroker) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceBroker) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceBrokersPath, Handler: h.create},
		{Method: "GET", Pattern: ServiceBrokersPath, Handler: h.list},
	}
}
