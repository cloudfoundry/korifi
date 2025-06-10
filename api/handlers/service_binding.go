package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	ServiceBindingsPath       = "/v3/service_credential_bindings"
	ServiceBindingPath        = "/v3/service_credential_bindings/{guid}"
	ServiceBindingDetailsPath = "/v3/service_credential_bindings/{guid}/details"
	ServiceBindingParamsPath  = "/v3/service_credential_bindings/{guid}/parameters"
)

type ServiceBinding struct {
	appRepo             CFAppRepository
	serviceBindingRepo  CFServiceBindingRepository
	serviceInstanceRepo CFServiceInstanceRepository
	serverURL           url.URL
	requestValidator    RequestValidator
}

//counterfeiter:generate -o fake -fake-name CFServiceBindingRepository . CFServiceBindingRepository
type CFServiceBindingRepository interface {
	CreateServiceBinding(context.Context, authorization.Info, repositories.CreateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
	DeleteServiceBinding(context.Context, authorization.Info, string) error
	ListServiceBindings(context.Context, authorization.Info, repositories.ListServiceBindingsMessage) ([]repositories.ServiceBindingRecord, error)
	GetServiceBinding(context.Context, authorization.Info, string) (repositories.ServiceBindingRecord, error)
	UpdateServiceBinding(context.Context, authorization.Info, repositories.UpdateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
	GetServiceBindingDetails(context.Context, authorization.Info, string) (repositories.ServiceBindingDetailsRecord, error)
	GetServiceBindingParameters(context.Context, authorization.Info, string) (map[string]any, error)
}

func NewServiceBinding(serverURL url.URL, serviceBindingRepo CFServiceBindingRepository, appRepo CFAppRepository, serviceInstanceRepo CFServiceInstanceRepository, requestValidator RequestValidator) *ServiceBinding {
	return &ServiceBinding{
		appRepo:             appRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
		serverURL:           serverURL,
		requestValidator:    requestValidator,
	}
}

func (h *ServiceBinding) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.create")

	var payload payloads.ServiceBindingCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, payload.Relationships.ServiceInstance.Data.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get "+repositories.ServiceInstanceResourceType)
	}

	ctx := logr.NewContext(r.Context(), logger.WithValues("service-instance", serviceInstance.GUID))

	if payload.Type == korifiv1alpha1.CFServiceBindingTypeApp {
		var app repositories.AppRecord
		if app, err = h.appRepo.GetApp(ctx, authInfo, payload.Relationships.App.Data.GUID); err != nil {
			return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get "+repositories.AppResourceType)
		}

		if app.SpaceGUID != serviceInstance.SpaceGUID {
			return nil, apierrors.LogAndReturn(
				logger,
				apierrors.NewUnprocessableEntityError(nil, "The service instance and the app are in different spaces"),
				"App and ServiceInstance in different spaces", "App GUID", app.GUID,
				"ServiceInstance GUID", serviceInstance.GUID,
			)
		}
	}

	if serviceInstance.Type == korifiv1alpha1.UserProvidedType {
		return h.createUserProvided(ctx, &payload, serviceInstance)
	}

	return h.createManaged(ctx, &payload, serviceInstance)
}

func (h *ServiceBinding) createUserProvided(ctx context.Context, payload *payloads.ServiceBindingCreate, serviceInstance repositories.ServiceInstanceRecord) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	logger := logr.FromContextOrDiscard(ctx).WithName("handlers.service-binding.create-user-provided")

	if payload.Type == korifiv1alpha1.CFServiceBindingTypeKey {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(nil, "Service credential bindings of type 'key' are not supported for user-provided service instances."),
			"",
		)
	}

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(serviceInstance.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logr.FromContextOrDiscard(ctx), err, "failed to create ServiceBinding")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) createManaged(ctx context.Context, payload *payloads.ServiceBindingCreate, serviceInstance repositories.ServiceInstanceRecord) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	logger := logr.FromContextOrDiscard(ctx).WithName("handlers.service-binding.create-managed")

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(serviceInstance.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create ServiceBinding")
	}
	return routing.NewResponse(http.StatusAccepted).
		WithHeader("Location", presenter.JobURLForRedirects(serviceBinding.GUID, presenter.ManagedServiceBindingCreateOperation, h.serverURL)), nil
}

func (h *ServiceBinding) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.delete")

	serviceBindingGUID := routing.URLParam(r, "guid")
	serviceBinding, err := h.serviceBindingRepo.GetServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get "+repositories.ServiceBindingResourceType)
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, serviceBinding.ServiceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(err, "failed to get service instance"),
			"failed to get "+repositories.ServiceInstanceResourceType,
			"instance-guid", serviceBinding.ServiceInstanceGUID,
		)
	}

	err = h.serviceBindingRepo.DeleteServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service binding", "guid", serviceBindingGUID)
	}

	if serviceInstance.Type == korifiv1alpha1.ManagedType {
		return routing.NewResponse(http.StatusAccepted).
			WithHeader("Location", presenter.JobURLForRedirects(serviceBinding.GUID, presenter.ManagedServiceBindingDeleteOperation, h.serverURL)), nil
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceBinding) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.list")

	listFilter := new(payloads.ServiceBindingList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, listFilter); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceBindingList, err := h.serviceBindingRepo.ListServiceBindings(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list "+repositories.ServiceBindingResourceType)
	}

	var appRecords []repositories.AppRecord
	if listFilter.Include != "" && len(serviceBindingList) > 0 {
		listAppsMessage := repositories.ListAppsMessage{}

		for _, serviceBinding := range serviceBindingList {
			listAppsMessage.Guids = append(listAppsMessage.Guids, serviceBinding.AppGUID)
		}

		appListResult, err := h.appRepo.ListApps(r.Context(), authInfo, listAppsMessage)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "failed to list "+repositories.AppResourceType)
		}
		appRecords = appListResult.Records
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBindingList(serviceBindingList, appRecords, h.serverURL, *r.URL)), nil
}

func (h *ServiceBinding) update(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.update")

	serviceBindingGUID := routing.URLParam(r, "guid")

	var payload payloads.ServiceBindingUpdate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	_, err := h.serviceBindingRepo.GetServiceBinding(r.Context(), authInfo, serviceBindingGUID)
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

func (h *ServiceBinding) getDetails(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.get-details")

	serviceBindingGUID := routing.URLParam(r, "guid")

	bindingDetails, err := h.serviceBindingRepo.GetServiceBindingDetails(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting service binding details in repository")
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBindingDetails(bindingDetails)), nil
}

func (h *ServiceBinding) getParameters(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.get")

	serviceBindingGUID := routing.URLParam(r, "guid")

	serviceBindingParams, err := h.serviceBindingRepo.GetServiceBindingParameters(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting service binding parameters from repository")
	}
	return routing.NewResponse(http.StatusOK).WithBody(serviceBindingParams), nil
}

func (h *ServiceBinding) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceBinding) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceBindingsPath, Handler: h.create},
		{Method: "GET", Pattern: ServiceBindingsPath, Handler: h.list},
		{Method: "GET", Pattern: ServiceBindingParamsPath, Handler: h.getParameters},
		{Method: "DELETE", Pattern: ServiceBindingPath, Handler: h.delete},
		{Method: "PATCH", Pattern: ServiceBindingPath, Handler: h.update},
		{Method: "GET", Pattern: ServiceBindingPath, Handler: h.get},
		{Method: "GET", Pattern: ServiceBindingDetailsPath, Handler: h.getDetails},
	}
}
