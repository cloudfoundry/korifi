package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	ServiceBindingsPath = "/v3/service_credential_bindings"
	ServiceBindingPath  = "/v3/service_credential_bindings/{guid}"
)

type ServiceBindingHandler struct {
	appRepo             CFAppRepository
	serviceBindingRepo  CFServiceBindingRepository
	serviceInstanceRepo CFServiceInstanceRepository
	serverURL           url.URL
	decoderValidator    *DecoderValidator
}

//counterfeiter:generate -o fake -fake-name CFServiceBindingRepository . CFServiceBindingRepository
type CFServiceBindingRepository interface {
	CreateServiceBinding(context.Context, authorization.Info, repositories.CreateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
	DeleteServiceBinding(context.Context, authorization.Info, string) error
	ListServiceBindings(context.Context, authorization.Info, repositories.ListServiceBindingsMessage) ([]repositories.ServiceBindingRecord, error)
}

func NewServiceBindingHandler(serverURL url.URL, serviceBindingRepo CFServiceBindingRepository, appRepo CFAppRepository, serviceInstanceRepo CFServiceInstanceRepository, decoderValidator *DecoderValidator) *ServiceBindingHandler {
	return &ServiceBindingHandler{
		appRepo:             appRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
		serverURL:           serverURL,
		decoderValidator:    decoderValidator,
	}
}

func (h *ServiceBindingHandler) createHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("service-binding-handler.create")

	var payload payloads.ServiceBindingCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
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

	return routing.NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBindingHandler) deleteHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("service-binding-handler.delete")

	serviceBindingGUID := chi.URLParam(r, "guid")

	err := h.serviceBindingRepo.DeleteServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service binding", "guid", serviceBindingGUID)
	}

	return routing.NewHandlerResponse(http.StatusNoContent), nil
}

func (h *ServiceBindingHandler) listHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("service-binding-handler.list")

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

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceBindingList(serviceBindingList, appRecords, h.serverURL, *r.URL)), nil
}

func (h *ServiceBindingHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("POST", ServiceBindingsPath, routing.Handler(h.createHandler))
	router.Method("GET", ServiceBindingsPath, routing.Handler(h.listHandler))
	router.Method("DELETE", ServiceBindingPath, routing.Handler(h.deleteHandler))
}
