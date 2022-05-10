package apis

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/schema"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/payloads"

	"code.cloudfoundry.org/korifi/api/presenter"

	"code.cloudfoundry.org/korifi/api/repositories"

	"code.cloudfoundry.org/korifi/api/authorization"

	"github.com/gorilla/mux"

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

type ServiceInstanceHandler struct {
	logger              logr.Logger
	serverURL           url.URL
	serviceInstanceRepo CFServiceInstanceRepository
	spaceRepo           SpaceRepository
	decoderValidator    *DecoderValidator
}

func NewServiceInstanceHandler(
	logger logr.Logger,
	serverURL url.URL,
	serviceInstanceRepo CFServiceInstanceRepository,
	spaceRepo SpaceRepository,
	decoderValidator *DecoderValidator,
) *ServiceInstanceHandler {
	return &ServiceInstanceHandler{
		logger:              logger,
		serverURL:           serverURL,
		serviceInstanceRepo: serviceInstanceRepo,
		spaceRepo:           spaceRepo,
		decoderValidator:    decoderValidator,
	}
}

//nolint:dupl
func (h *ServiceInstanceHandler) serviceInstanceCreateHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := context.Background()

	var payload payloads.ServiceInstanceCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch namespace from Kubernetes", "spaceGUID", spaceGUID)
		return nil, apierrors.AsUnprocessableEntity(err, "Invalid space. Ensure that the space exists and you have access to it.", apierrors.NotFoundError{}, apierrors.ForbiddenError{})
	}

	serviceInstanceRecord, err := h.serviceInstanceRepo.CreateServiceInstance(ctx, authInfo, payload.ToServiceInstanceCreateMessage())
	if err != nil {
		h.logger.Error(err, "Failed to create service instance", "Service Instance Name", serviceInstanceRecord.Name)
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForServiceInstance(serviceInstanceRecord, h.serverURL)), nil
}

func (h *ServiceInstanceHandler) serviceInstanceListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := context.Background()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	for k := range r.Form {
		if strings.HasPrefix(k, "fields[") || k == "per_page" {
			r.Form.Del(k)
		}
	}

	listFilter := new(payloads.ServiceInstanceList)
	err := schema.NewDecoder().Decode(listFilter, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in ServiceInstance filter")
					return nil, apierrors.NewUnknownKeyError(err, listFilter.SupportedFilterKeys())
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	serviceInstanceList, err := h.serviceInstanceRepo.ListServiceInstances(ctx, authInfo, listFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to list service instance")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceInstanceList(serviceInstanceList, h.serverURL, *r.URL)), nil
}

func (h *ServiceInstanceHandler) serviceInstanceDeleteHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	serviceInstanceGUID := vars["guid"]

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, serviceInstanceGUID)
	if err != nil {
		h.logger.Error(err, "failed to get service instance")
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	err = h.serviceInstanceRepo.DeleteServiceInstance(ctx, authInfo, repositories.DeleteServiceInstanceMessage{
		GUID:      serviceInstanceGUID,
		SpaceGUID: serviceInstance.SpaceGUID,
	})
	if err != nil {
		h.logger.Error(err, "error when deleting service instance", "guid", serviceInstanceGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusNoContent), nil
}

func (h *ServiceInstanceHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ServiceInstancesPath).Methods(http.MethodPost).HandlerFunc(w.Wrap(h.serviceInstanceCreateHandler))
	router.Path(ServiceInstancesPath).Methods(http.MethodGet).HandlerFunc(w.Wrap(h.serviceInstanceListHandler))
	router.Path(ServiceInstancePath).Methods(http.MethodDelete).HandlerFunc(w.Wrap(h.serviceInstanceDeleteHandler))
}
