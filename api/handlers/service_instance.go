package handlers

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"sort"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/routing"
	"golang.org/x/exp/maps"

	"code.cloudfoundry.org/korifi/api/presenter"

	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"code.cloudfoundry.org/korifi/api/authorization"

	"github.com/go-logr/logr"
)

const (
	ServiceInstancesPath          = "/v3/service_instances"
	ServiceInstancePath           = "/v3/service_instances/{guid}"
	ServiceInstanceParametersPath = ServiceInstancePath + "/parameters"
)

//counterfeiter:generate -o fake -fake-name CFServiceInstanceRepository . CFServiceInstanceRepository
type CFServiceInstanceRepository interface {
	CreateServiceInstance(context.Context, authorization.Info, repositories.CreateServiceInstanceMessage) (repositories.ServiceInstanceRecord, error)
	PatchServiceInstance(context.Context, authorization.Info, repositories.PatchServiceInstanceMessage) (repositories.ServiceInstanceRecord, error)
	ListServiceInstances(context.Context, authorization.Info, repositories.ListServiceInstanceMessage) ([]repositories.ServiceInstanceRecord, error)
	GetServiceInstance(context.Context, authorization.Info, string) (repositories.ServiceInstanceRecord, error)
	DeleteServiceInstance(context.Context, authorization.Info, repositories.DeleteServiceInstanceMessage) error
}

type ServiceInstance struct {
	serverURL           url.URL
	serviceInstanceRepo CFServiceInstanceRepository
	spaceRepo           CFSpaceRepository
	brokerRepo          CFServiceBrokerRepository
	serviceCatalogRepo  ServiceCatalogRepo
	requestValidator    RequestValidator
}

func NewServiceInstance(
	serverURL url.URL,
	serviceInstanceRepo CFServiceInstanceRepository,
	spaceRepo CFSpaceRepository,
	brokerRepo CFServiceBrokerRepository,
	serviceCatalogRepo ServiceCatalogRepo,
	requestValidator RequestValidator,
) *ServiceInstance {
	return &ServiceInstance{
		serverURL:           serverURL,
		serviceInstanceRepo: serviceInstanceRepo,
		spaceRepo:           spaceRepo,
		brokerRepo:          brokerRepo,
		serviceCatalogRepo:  serviceCatalogRepo,
		requestValidator:    requestValidator,
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

	serviceInstanceRecord, err := h.serviceInstanceRepo.CreateServiceInstance(r.Context(), authInfo, payload.ToServiceInstanceCreateMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create service instance", "Service Instance Name", serviceInstanceRecord.Name)
	}
	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(serviceInstanceRecord.GUID, presenter.ServiceInstanceCreateOperation, h.serverURL)), nil
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

	listFilter := new(payloads.ServiceInstanceList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, listFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceInstanceList, err := h.serviceInstanceRepo.ListServiceInstances(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service instance")
	}

	h.sortList(serviceInstanceList, listFilter.OrderBy)

	includedBrokers := map[string]any{}
	includedOfferings := map[string]any{}
	includedPlans := map[string]any{}
	for _, si := range serviceInstanceList {
		if si.Type != korifiv1alpha1.ManagedType {
			continue
		}

		plan, err := h.serviceCatalogRepo.GetServicePlan(r.Context(), authInfo, si.PlanGUID)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "Failed to get service plan", "guid", si.PlanGUID)
		}
		planDetails := map[string]any{}
		if slices.Contains(listFilter.IncludeServicePlans, "guid") {
			planDetails["guid"] = plan.GUID
		}
		if slices.Contains(listFilter.IncludeServicePlans, "name") {
			planDetails["name"] = plan.Name
		}
		if slices.Contains(listFilter.IncludeServicePlans, "relationships.service_offering") {
			planDetails["relationships"] = map[string]any{
				"service_offering": plan.Relationships.Service_offering,
			}
		}
		includedPlans[plan.GUID] = planDetails

		offering, err := h.serviceCatalogRepo.GetServiceOffering(r.Context(), authInfo, plan.Relationships.Service_offering.Data.GUID)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "Failed to get service offering", "guid", plan.Relationships.Service_offering.Data.GUID)
		}
		offeringDetails := map[string]any{}
		if slices.Contains(listFilter.IncludeServiceOfferings, "guid") {
			offeringDetails["guid"] = offering.GUID
		}
		if slices.Contains(listFilter.IncludeServiceOfferings, "name") {
			offeringDetails["name"] = offering.Name
		}
		if slices.Contains(listFilter.IncludeServiceOfferings, "relationships.service_broker") {
			offeringDetails["relationships"] = map[string]any{
				"service_broker": offering.Relationships.Service_broker,
			}
		}
		includedOfferings[offering.GUID] = offeringDetails

		broker, err := h.brokerRepo.GetServiceBroker(r.Context(), authInfo, offering.Relationships.Service_broker.Data.GUID)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "Failed to get service broker", "guid", offering.Relationships.Service_broker.Data.GUID)
		}
		brokerDetails := map[string]any{}
		if slices.Contains(listFilter.IncludeServiceBrokers, "guid") {
			brokerDetails["guid"] = broker.GUID
		}
		if slices.Contains(listFilter.IncludeServiceBrokers, "name") {
			brokerDetails["name"] = broker.Name
		}
		includedBrokers[broker.GUID] = brokerDetails
	}

	includedResources := []presenter.IncludedResources{}
	if len(listFilter.IncludeServiceBrokers) > 0 {
		includedResources = append(includedResources, presenter.IncludedResources{
			Type:      "service_brokers",
			Resources: maps.Values(includedBrokers),
		})
	}

	if len(listFilter.IncludeServiceOfferings) > 0 {
		includedResources = append(includedResources, presenter.IncludedResources{
			Type:      "service_offerings",
			Resources: maps.Values(includedOfferings),
		})
	}

	if len(listFilter.IncludeServicePlans) > 0 {
		includedResources = append(includedResources, presenter.IncludedResources{
			Type:      "service_plans",
			Resources: maps.Values(includedPlans),
		})
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForServiceInstance, serviceInstanceList, h.serverURL, *r.URL, includedResources...)), nil
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

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceInstance) getParameters(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.get-paramaters")

	serviceInstanceGUID := routing.URLParam(r, "guid")

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(r.Context(), authInfo, serviceInstanceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get service instance")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceInstanceParameters(serviceInstance)), nil
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
		{Method: "GET", Pattern: ServiceInstanceParametersPath, Handler: h.getParameters},
	}
}
