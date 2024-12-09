// nolint:dupl
package handlers

import (
	"context"
	"log"
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
	ServiceOfferingsPath = "/v3/service_offerings"
	ServiceOfferingPath  = "/v3/service_offerings/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFServiceOfferingRepository . CFServiceOfferingRepository
type CFServiceOfferingRepository interface {
	GetServiceOffering(context.Context, authorization.Info, string) (repositories.ServiceOfferingRecord, error)
	ListOfferings(context.Context, authorization.Info, repositories.ListServiceOfferingMessage) ([]repositories.ServiceOfferingRecord, error)
	DeleteOffering(context.Context, authorization.Info, repositories.DeleteServiceOfferingMessage) error
}

type ServiceOffering struct {
	serverURL           url.URL
	requestValidator    RequestValidator
	serviceOfferingRepo CFServiceOfferingRepository
	serviceBrokerRepo   CFServiceBrokerRepository
	includeResolver     *include.IncludeResolver[
		[]repositories.ServiceOfferingRecord,
		repositories.ServiceOfferingRecord,
	]
}

func NewServiceOffering(
	serverURL url.URL,
	requestValidator RequestValidator,
	serviceOfferingRepo CFServiceOfferingRepository,
	serviceBrokerRepo CFServiceBrokerRepository,
	relationshipRepo include.ResourceRelationshipRepository,
) *ServiceOffering {
	return &ServiceOffering{
		serverURL:           serverURL,
		requestValidator:    requestValidator,
		serviceOfferingRepo: serviceOfferingRepo,
		serviceBrokerRepo:   serviceBrokerRepo,
		includeResolver:     include.NewIncludeResolver[[]repositories.ServiceOfferingRecord](relationshipRepo, presenter.NewResource(serverURL)),
	}
}

func (h *ServiceOffering) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-offering.create")

	payload := new(payloads.ServiceOfferingGet)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceOfferingGUID := routing.URLParam(r, "guid")

	serviceOffering, err := h.serviceOfferingRepo.GetServiceOffering(r.Context(), authInfo, serviceOfferingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get service offering: %s", serviceOfferingGUID)
	}

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, []repositories.ServiceOfferingRecord{serviceOffering}, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	log.Printf("included: %+v", includedResources)

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceOffering(serviceOffering, h.serverURL, includedResources...)), nil
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

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, serviceOfferingList, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceOffering, serviceOfferingList, h.serverURL, *r.URL, includedResources...)), nil
}

func (h *ServiceOffering) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-offering.delete")

	payload := new(payloads.ServiceOfferingDelete)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceOfferingGUID := routing.URLParam(r, "guid")
	if err := h.serviceOfferingRepo.DeleteOffering(r.Context(), authInfo, payload.ToMessage(serviceOfferingGUID)); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to delete service offering: %s", serviceOfferingGUID)
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceOffering) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceOffering) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServiceOfferingPath, Handler: h.get},
		{Method: "GET", Pattern: ServiceOfferingsPath, Handler: h.list},
		{Method: "DELETE", Pattern: ServiceOfferingPath, Handler: h.delete},
	}
}
