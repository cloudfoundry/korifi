package handlers

import (
	"context"
	"net/http"
	"net/url"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/api/routing"

	"code.cloudfoundry.org/korifi/api/presenter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"code.cloudfoundry.org/korifi/api/repositories"

	"code.cloudfoundry.org/korifi/api/authorization"

	"github.com/go-logr/logr"
)

const (
	ServiceInstancesPath           = "/v3/service_instances"
	ServiceInstancePath            = "/v3/service_instances/{guid}"
	ServiceInstanceCredentialsPath = "/v3/service_instances/{guid}/credentials"
)

//counterfeiter:generate -o fake -fake-name CFServiceInstanceRepository . CFServiceInstanceRepository
type CFServiceInstanceRepository interface {
	CreateUserProvidedServiceInstance(context.Context, authorization.Info, repositories.CreateUPSIMessage) (repositories.ServiceInstanceRecord, error)
	CreateManagedServiceInstance(context.Context, authorization.Info, repositories.CreateManagedSIMessage) (repositories.ServiceInstanceRecord, error)
	PatchUserProvidedServiceInstance(context.Context, authorization.Info, repositories.PatchUPSIMessage) (repositories.ServiceInstanceRecord, error)
	PatchManagedServiceInstance(context.Context, authorization.Info, repositories.PatchManagedSIMessage) (repositories.ServiceInstanceRecord, error)
	ListServiceInstances(context.Context, authorization.Info, repositories.ListServiceInstanceMessage) (repositories.ListResult[repositories.ServiceInstanceRecord], error)
	GetServiceInstance(context.Context, authorization.Info, string) (repositories.ServiceInstanceRecord, error)
	GetServiceInstanceCredentials(context.Context, authorization.Info, string) (map[string]any, error)
	DeleteServiceInstance(context.Context, authorization.Info, repositories.DeleteServiceInstanceMessage) (repositories.ServiceInstanceRecord, error)
}

type ServiceInstance struct {
	serverURL           url.URL
	serviceInstanceRepo CFServiceInstanceRepository
	spaceRepo           CFSpaceRepository
	requestValidator    RequestValidator
	includeResolver     *include.IncludeResolver[
		[]repositories.ServiceInstanceRecord,
		repositories.ServiceInstanceRecord,
	]
}

func NewServiceInstance(
	serverURL url.URL,
	serviceInstanceRepo CFServiceInstanceRepository,
	spaceRepo CFSpaceRepository,
	requestValidator RequestValidator,
	relationshipRepo include.ResourceRelationshipRepository,
) *ServiceInstance {
	return &ServiceInstance{
		serverURL:           serverURL,
		serviceInstanceRepo: serviceInstanceRepo,
		spaceRepo:           spaceRepo,
		requestValidator:    requestValidator,
		includeResolver:     include.NewIncludeResolver[[]repositories.ServiceInstanceRecord](relationshipRepo, presenter.NewResource(serverURL)),
	}
}

func (h *ServiceInstance) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.get")

	payload := new(payloads.ServiceInstanceGet)
	err := h.requestValidator.DecodeAndValidateURLValues(r, payload)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceInstanceGUID := routing.URLParam(r, "guid")

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, serviceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service instance", "GUID", serviceInstanceGUID)
	}

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, []repositories.ServiceInstanceRecord{serviceInstance}, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceInstance(serviceInstance, h.serverURL, includedResources...)), nil
}

func (h *ServiceInstance) getCredentials(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.get-credentials")

	serviceInstanceGUID := routing.URLParam(r, "guid")

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, serviceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service instance", "GUID", serviceInstanceGUID)
	}

	if serviceInstance.Type != korifiv1alpha1.UserProvidedType {
		return nil, apierrors.NewNotFoundError(nil, repositories.ServiceInstanceResourceType)
	}

	credentials, err := h.serviceInstanceRepo.GetServiceInstanceCredentials(r.Context(), authInfo, serviceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get service instance credentials")
	}

	return routing.NewResponse(http.StatusOK).WithBody(credentials), nil
}

//nolint:dupl
func (h *ServiceInstance) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.create")

	var payload payloads.ServiceInstanceCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, "Invalid space. Ensure that the space exists and you have access to it.", apierrors.NotFoundError{}, apierrors.ForbiddenError{}),
			"Failed to fetch namespace from Kubernetes",
			"spaceGUID", spaceGUID,
		)
	}

	if payload.Type == korifiv1alpha1.ManagedType {
		return h.createManagedServiceInstance(r.Context(), logger, authInfo, payload)
	}

	return h.createUserProvidedServiceInstance(r.Context(), logger, authInfo, payload)
}

func (h *ServiceInstance) createManagedServiceInstance(
	ctx context.Context,
	logger logr.Logger,
	authInfo authorization.Info,
	payload payloads.ServiceInstanceCreate,
) (*routing.Response, error) {
	serviceInstanceRecord, err := h.serviceInstanceRepo.CreateManagedServiceInstance(ctx, authInfo, payload.ToManagedSICreateMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create managed service instance", "Service Instance Name", payload.Name)
	}

	return routing.NewResponse(http.StatusAccepted).
		WithHeader("Location", presenter.JobURLForRedirects(serviceInstanceRecord.GUID, presenter.ManagedServiceInstanceCreateOperation, h.serverURL)), nil
}

func (h *ServiceInstance) createUserProvidedServiceInstance(
	ctx context.Context,
	logger logr.Logger,
	authInfo authorization.Info,
	payload payloads.ServiceInstanceCreate,
) (*routing.Response, error) {
	serviceInstanceRecord, err := h.serviceInstanceRepo.CreateUserProvidedServiceInstance(ctx, authInfo, payload.ToUPSICreateMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create user provided service instance", "Service Instance Name", payload.Name)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForServiceInstance(serviceInstanceRecord, h.serverURL)), nil
}

func (h *ServiceInstance) patch(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.patch")

	var payload payloads.ServiceInstancePatch
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	serviceInstanceGUID := routing.URLParam(r, "guid")

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, serviceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service instance")
	}

	if payload.Type == korifiv1alpha1.ManagedType {
		patchMessage := payload.ToManagedSIPatchMessage(serviceInstance.SpaceGUID, serviceInstance.GUID)
		serviceInstance, err = h.serviceInstanceRepo.PatchManagedServiceInstance(r.Context(), authInfo, patchMessage)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "failed to patch managed service instance")
		}

		return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceInstance(serviceInstance, h.serverURL)), nil
	}

	patchMessage := payload.ToUPSIPatchMessage(serviceInstance.SpaceGUID, serviceInstance.GUID)
	serviceInstance, err = h.serviceInstanceRepo.PatchUserProvidedServiceInstance(r.Context(), authInfo, patchMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to patch user provided service instance")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceInstance(serviceInstance, h.serverURL)), nil
}

func (h *ServiceInstance) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.list")

	payload := new(payloads.ServiceInstanceList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, payload)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceInstances, err := h.serviceInstanceRepo.ListServiceInstances(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service instance")
	}

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, serviceInstances.Records, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceInstance, serviceInstances, h.serverURL, *r.URL, includedResources...)), nil
}

func (h *ServiceInstance) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.delete")

	serviceInstanceGUID := routing.URLParam(r, "guid")

	payload := new(payloads.ServiceInstanceDelete)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceInstance, err := h.serviceInstanceRepo.DeleteServiceInstance(r.Context(), authInfo, payload.ToMessage(serviceInstanceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service instance", "guid", serviceInstanceGUID)
	}

	if serviceInstance.Type == korifiv1alpha1.ManagedType {
		return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(serviceInstance.GUID, presenter.ManagedServiceInstanceDeleteOperation, h.serverURL)), nil
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceInstance) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceInstance) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceInstancesPath, Handler: h.create},
		{Method: "PATCH", Pattern: ServiceInstancePath, Handler: h.patch},
		{Method: "GET", Pattern: ServiceInstancesPath, Handler: h.list},
		{Method: "GET", Pattern: ServiceInstancePath, Handler: h.get},
		{Method: "GET", Pattern: ServiceInstanceCredentialsPath, Handler: h.getCredentials},
		{Method: "DELETE", Pattern: ServiceInstancePath, Handler: h.delete},
	}
}
