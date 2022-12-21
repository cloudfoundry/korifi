package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	ServiceBindingsPath = "/v3/service_credential_bindings"
	ServiceBindingPath  = "/v3/service_credential_bindings/{guid}"
)

type ServiceBindingHandler struct {
	handlerWrapper      *AuthAwareHandlerFuncWrapper
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
		handlerWrapper:      NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("ServiceBindingHandler")),
		appRepo:             appRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
		serverURL:           serverURL,
		decoderValidator:    decoderValidator,
	}
}

func (h *ServiceBindingHandler) createHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.ServiceBindingCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to get %s", repositories.AppResourceType))
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, payload.Relationships.ServiceInstance.Data.GUID)
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

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create ServiceBinding", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBindingHandler) deleteHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	serviceBindingGUID := chi.URLParam(r, "guid")

	err := h.serviceBindingRepo.DeleteServiceBinding(ctx, authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service binding", "guid", serviceBindingGUID)
	}

	return NewHandlerResponse(http.StatusNoContent), nil
}

func (h *ServiceBindingHandler) listHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewUnprocessableEntityError(err, "unable to parse query"), "Unable to parse request query parameters")
	}

	listFilter := new(payloads.ServiceBindingList)
	err := payloads.Decode(listFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceBindingList, err := h.serviceBindingRepo.ListServiceBindings(ctx, authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to list %s", repositories.ServiceBindingResourceType))
	}

	var appRecords []repositories.AppRecord
	if listFilter.Include != "" && len(serviceBindingList) > 0 {
		listAppsMessage := repositories.ListAppsMessage{}

		for _, serviceBinding := range serviceBindingList {
			listAppsMessage.Guids = append(listAppsMessage.Guids, serviceBinding.AppGUID)
		}

		appRecords, err = h.appRepo.ListApps(ctx, authInfo, listAppsMessage)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, fmt.Sprintf("failed to list %s", repositories.AppResourceType))
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceBindingList(serviceBindingList, appRecords, h.serverURL, *r.URL)), nil
}

func (h *ServiceBindingHandler) RegisterRoutes(router *chi.Mux) {
	router.Post(ServiceBindingsPath, h.handlerWrapper.Wrap(h.createHandler))
	router.Get(ServiceBindingsPath, h.handlerWrapper.Wrap(h.listHandler))
	router.Delete(ServiceBindingPath, h.handlerWrapper.Wrap(h.deleteHandler))
}
