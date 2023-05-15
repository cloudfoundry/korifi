package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	ServiceBindingsPath = "/v3/service_credential_bindings"
	ServiceBindingPath  = "/v3/service_credential_bindings/{guid}"
)

type ServiceBinding struct {
	appRepo             CFAppRepository
	serviceBindingRepo  CFServiceBindingRepository
	serviceInstanceRepo CFServiceInstanceRepository
	serverURL           url.URL
	payloadValidator    *GoPlaygroundValidator
}

//counterfeiter:generate -o fake -fake-name CFServiceBindingRepository . CFServiceBindingRepository
type CFServiceBindingRepository interface {
	CreateServiceBinding(context.Context, authorization.Info, repositories.CreateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
	DeleteServiceBinding(context.Context, authorization.Info, string) error
	ListServiceBindings(context.Context, authorization.Info, repositories.ListServiceBindingsMessage) ([]repositories.ServiceBindingRecord, error)
	GetServiceBinding(context.Context, authorization.Info, string) (repositories.ServiceBindingRecord, error)
	UpdateServiceBinding(context.Context, authorization.Info, repositories.UpdateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
}

func NewServiceBinding(serverURL url.URL, serviceBindingRepo CFServiceBindingRepository, appRepo CFAppRepository, serviceInstanceRepo CFServiceInstanceRepository, payloadValidator *GoPlaygroundValidator) *ServiceBinding {
	return &ServiceBinding{
		appRepo:             appRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
		serverURL:           serverURL,
		payloadValidator:    payloadValidator,
	}
}

func (h *ServiceBinding) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.create")

	payload, err := BodyToObject[payloads.ServiceBindingCreate](r, true)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if err := h.payloadValidator.ValidatePayload(payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to validate payload")
	}

	app, err := h.appRepo.GetApp(r.Context(), authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to get %s", repositories.AppResourceType))
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, payload.Relationships.ServiceInstance.Data.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to get %s", repositories.ServiceInstanceResourceType))
	}

	if app.SpaceGUID != serviceInstance.SpaceGUID {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(nil, "The service instance and the app are in different spaces"),
			"App and ServiceInstance in different spaces", "App GUID", app.GUID,
			"ServiceInstance GUID", serviceInstance.GUID,
		)
	}

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(r.Context(), authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create ServiceBinding", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.delete")

	serviceBindingGUID := routing.URLParam(r, "guid")

	err := h.serviceBindingRepo.DeleteServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service binding", "guid", serviceBindingGUID)
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceBinding) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewUnprocessableEntityError(err, "unable to parse query"), "Unable to parse request query parameters")
	}

	listFilter := new(payloads.ServiceBindingList)
	err := payloads.Decode(listFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceBindingList, err := h.serviceBindingRepo.ListServiceBindings(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to list %s", repositories.ServiceBindingResourceType))
	}

	var appRecords []repositories.AppRecord
	if listFilter.Include != "" && len(serviceBindingList) > 0 {
		listAppsMessage := repositories.ListAppsMessage{}

		for _, serviceBinding := range serviceBindingList {
			listAppsMessage.Guids = append(listAppsMessage.Guids, serviceBinding.AppGUID)
		}

		appRecords, err = h.appRepo.ListApps(r.Context(), authInfo, listAppsMessage)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to list %s", repositories.AppResourceType))
		}
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBindingList(serviceBindingList, appRecords, h.serverURL, *r.URL)), nil
}

func (h *ServiceBinding) update(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.update")

	serviceBindingGUID := routing.URLParam(r, "guid")

	payload, err := BodyToObject[payloads.ServiceBindingUpdate](r, true)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if err := h.payloadValidator.ValidatePayload(payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to validate payload")
	}

	_, err = h.serviceBindingRepo.GetServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting service binding in repository")
	}

	serviceBinding, err := h.serviceBindingRepo.UpdateServiceBinding(r.Context(), authInfo, payload.ToMessage(serviceBindingGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating service binding in repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.get")

	serviceBindingGUID := routing.URLParam(r, "guid")

	serviceBinding, err := h.serviceBindingRepo.GetServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting service binding in repository")
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceBinding) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceBindingsPath, Handler: h.create},
		{Method: "GET", Pattern: ServiceBindingsPath, Handler: h.list},
		{Method: "DELETE", Pattern: ServiceBindingPath, Handler: h.delete},
		{Method: "PATCH", Pattern: ServiceBindingPath, Handler: h.update},
		{Method: "GET", Pattern: ServiceBindingPath, Handler: h.get},
	}
}
