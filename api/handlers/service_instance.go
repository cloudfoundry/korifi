package handlers

import (
	"context"
	"net/http"
	"net/url"
	"sort"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers/include"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/routing"

	"code.cloudfoundry.org/korifi/api/presenter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"code.cloudfoundry.org/korifi/api/repositories"

	"code.cloudfoundry.org/korifi/api/authorization"

	"github.com/go-logr/logr"
)

const (
	ServiceInstancesPath = "/v3/service_instances"
	ServiceInstancePath  = "/v3/service_instances/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFServiceInstanceRepository . CFServiceInstanceRepository
type CFServiceInstanceRepository interface {
	CreateUserProvidedServiceInstance(context.Context, authorization.Info, repositories.CreateUPSIMessage) (repositories.ServiceInstanceRecord, error)
	CreateManagedServiceInstance(context.Context, authorization.Info, repositories.CreateManagedSIMessage) (repositories.ServiceInstanceRecord, error)
	PatchServiceInstance(context.Context, authorization.Info, repositories.PatchServiceInstanceMessage) (repositories.ServiceInstanceRecord, error)
	ListServiceInstances(context.Context, authorization.Info, repositories.ListServiceInstanceMessage) ([]repositories.ServiceInstanceRecord, error)
	GetServiceInstance(context.Context, authorization.Info, string) (repositories.ServiceInstanceRecord, error)
	DeleteServiceInstance(context.Context, authorization.Info, repositories.DeleteServiceInstanceMessage) error
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

	if payload.Type == "managed" {
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

	patchMessage := payload.ToServiceInstancePatchMessage(serviceInstance.SpaceGUID, serviceInstance.GUID)
	serviceInstance, err = h.serviceInstanceRepo.PatchServiceInstance(r.Context(), authInfo, patchMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to patch service instance")
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

	h.sortList(serviceInstances, payload.OrderBy)

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, serviceInstances, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceInstance, serviceInstances, h.serverURL, *r.URL, includedResources...)), nil
}

// nolint:dupl
func (h *ServiceInstance) sortList(siList []repositories.ServiceInstanceRecord, order string) {
	switch order {
	case "":
	case "created_at":
		sort.Slice(siList, func(i, j int) bool { return timePtrAfter(&siList[j].CreatedAt, &siList[i].CreatedAt) })
	case "-created_at":
		sort.Slice(siList, func(i, j int) bool { return timePtrAfter(&siList[i].CreatedAt, &siList[j].CreatedAt) })
	case "updated_at":
		sort.Slice(siList, func(i, j int) bool { return timePtrAfter(siList[j].UpdatedAt, siList[i].UpdatedAt) })
	case "-updated_at":
		sort.Slice(siList, func(i, j int) bool { return timePtrAfter(siList[i].UpdatedAt, siList[j].UpdatedAt) })
	case "name":
		sort.Slice(siList, func(i, j int) bool { return siList[i].Name < siList[j].Name })
	case "-name":
		sort.Slice(siList, func(i, j int) bool { return siList[i].Name > siList[j].Name })
	}
}

func (h *ServiceInstance) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.delete")

	serviceInstanceGUID := routing.URLParam(r, "guid")

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, serviceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service instance")
	}

	err = h.serviceInstanceRepo.DeleteServiceInstance(r.Context(), authInfo, repositories.DeleteServiceInstanceMessage{
		GUID:      serviceInstanceGUID,
		SpaceGUID: serviceInstance.SpaceGUID,
	})
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
		{Method: "DELETE", Pattern: ServiceInstancePath, Handler: h.delete},
	}
}
