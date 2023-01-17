package handlers

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/routing"

	"code.cloudfoundry.org/korifi/api/presenter"

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
	CreateServiceInstance(context.Context, authorization.Info, repositories.CreateServiceInstanceMessage) (repositories.ServiceInstanceRecord, error)
	ListServiceInstances(context.Context, authorization.Info, repositories.ListServiceInstanceMessage) ([]repositories.ServiceInstanceRecord, error)
	GetServiceInstance(context.Context, authorization.Info, string) (repositories.ServiceInstanceRecord, error)
	DeleteServiceInstance(context.Context, authorization.Info, repositories.DeleteServiceInstanceMessage) error
}

type ServiceInstance struct {
	serverURL           url.URL
	serviceInstanceRepo CFServiceInstanceRepository
	spaceRepo           SpaceRepository
	decoderValidator    *DecoderValidator
}

func NewServiceInstance(
	serverURL url.URL,
	serviceInstanceRepo CFServiceInstanceRepository,
	spaceRepo SpaceRepository,
	decoderValidator *DecoderValidator,
) *ServiceInstance {
	return &ServiceInstance{
		serverURL:           serverURL,
		serviceInstanceRepo: serviceInstanceRepo,
		spaceRepo:           spaceRepo,
		decoderValidator:    decoderValidator,
	}
}

//nolint:dupl
func (h *ServiceInstance) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.create")

	var payload payloads.ServiceInstanceCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForServiceInstance(serviceInstanceRecord, h.serverURL)), nil
}

func (h *ServiceInstance) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-instance.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	for k := range r.Form {
		if strings.HasPrefix(k, "fields[") || k == "per_page" {
			r.Form.Del(k)
		}
	}

	listFilter := new(payloads.ServiceInstanceList)
	err := payloads.Decode(listFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceInstanceList, err := h.serviceInstanceRepo.ListServiceInstances(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to list service instance")
	}

	if err := h.sortList(serviceInstanceList, r.FormValue("order_by")); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "bad order by value")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceInstanceList(serviceInstanceList, h.serverURL, *r.URL)), nil
}

// nolint:dupl
func (h *ServiceInstance) sortList(siList []repositories.ServiceInstanceRecord, order string) error {
	switch order {
	case "":
	case "created_at":
		sort.Slice(siList, func(i, j int) bool { return siList[i].CreatedAt < siList[j].CreatedAt })
	case "-created_at":
		sort.Slice(siList, func(i, j int) bool { return siList[i].CreatedAt > siList[j].CreatedAt })
	case "updated_at":
		sort.Slice(siList, func(i, j int) bool { return siList[i].UpdatedAt < siList[j].UpdatedAt })
	case "-updated_at":
		sort.Slice(siList, func(i, j int) bool { return siList[i].UpdatedAt > siList[j].UpdatedAt })
	case "name":
		sort.Slice(siList, func(i, j int) bool { return siList[i].Name < siList[j].Name })
	case "-name":
		sort.Slice(siList, func(i, j int) bool { return siList[i].Name > siList[j].Name })
	default:
		return apierrors.NewBadQueryParamValueError("Order by", "created_at", "updated_at", "name")
	}
	return nil
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

func (h *ServiceInstance) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceInstance) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceInstancesPath, Handler: h.create},
		{Method: "GET", Pattern: ServiceInstancesPath, Handler: h.list},
		{Method: "DELETE", Pattern: ServiceInstancePath, Handler: h.delete},
	}
}
