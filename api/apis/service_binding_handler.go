package apis

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"

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
	logger              logr.Logger
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
	ServiceBindingExists(ctx context.Context, info authorization.Info, spaceGUID, appGUID, serviceInsanceGUID string) (bool, error)
	ListServiceBindings(context.Context, authorization.Info, repositories.ListServiceBindingsMessage) ([]repositories.ServiceBindingRecord, error)
}

func NewServiceBindingHandler(logger logr.Logger, serverURL url.URL, serviceBindingRepo CFServiceBindingRepository, appRepo CFAppRepository, serviceInstanceRepo CFServiceInstanceRepository, decoderValidator *DecoderValidator) *ServiceBindingHandler {
	return &ServiceBindingHandler{
		logger:              logger,
		appRepo:             appRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
		serverURL:           serverURL,
		decoderValidator:    decoderValidator,
	}
}

func (h *ServiceBindingHandler) createHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	var payload payloads.ServiceBindingCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		h.logger.Error(err, "failed to get %s", repositories.AppResourceType)
		return nil, err
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, payload.Relationships.ServiceInstance.Data.GUID)
	if err != nil {
		h.logger.Error(err, "failed to get %s", repositories.ServiceInstanceResourceType)
		return nil, err
	}

	if app.SpaceGUID != serviceInstance.SpaceGUID {
		h.logger.Info("App and ServiceInstance in different spaces", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
		return nil, apierrors.NewUnprocessableEntityError(err, "The service instance and the app are in different spaces")
	}

	bindingExists, err := h.serviceBindingRepo.ServiceBindingExists(ctx, authInfo, app.SpaceGUID, app.GUID, serviceInstance.GUID)
	if err != nil {
		h.logger.Error(err, "failed to get %s", repositories.ServiceBindingResourceType)
		return nil, err
	}
	if bindingExists {
		h.logger.Info("ServiceBinding already exists for App and ServiceInstance", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
		return nil, apierrors.NewUnprocessableEntityError(err, "The app is already bound to the service instance")
	}

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		h.logger.Error(err, "failed to create %s", repositories.ServiceBindingResourceType)
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBindingHandler) deleteHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	serviceBindingGUID := vars["guid"]

	err := h.serviceBindingRepo.DeleteServiceBinding(ctx, authInfo, serviceBindingGUID)
	if err != nil {
		h.logger.Error(err, "error when deleting service binding", "guid", serviceBindingGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusNoContent).WithBody(map[string]interface{}{}), nil
}

func (h *ServiceBindingHandler) listHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := context.Background()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, apierrors.NewUnprocessableEntityError(err, "unable to parse query")
	}

	listFilter := new(payloads.ServiceBindingList)
	err := schema.NewDecoder().Decode(listFilter, r.Form)
	if err != nil {
		if isUnknownKeyError(err) {
			h.logger.Info("Unknown key used in ServiceInstance query parameters")
			return nil, apierrors.NewUnknownKeyError(err, listFilter.SupportedFilterKeys())
		} else {
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	serviceBindingList, err := h.serviceBindingRepo.ListServiceBindings(ctx, authInfo, listFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "failed to list %s", repositories.ServiceBindingResourceType)
		return nil, err
	}

	var appRecords []repositories.AppRecord
	if listFilter.Include != nil && len(serviceBindingList) > 0 {
		listAppsMessage := repositories.ListAppsMessage{}

		for _, serviceBinding := range serviceBindingList {
			listAppsMessage.Guids = append(listAppsMessage.Guids, serviceBinding.AppGUID)
		}

		appRecords, err = h.appRepo.ListApps(ctx, authInfo, listAppsMessage)
		if err != nil {
			h.logger.Error(err, "failed to list %s", repositories.AppResourceType)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceBindingList(serviceBindingList, appRecords, h.serverURL, *r.URL)), nil
}

func (h *ServiceBindingHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ServiceBindingsPath).Methods("POST").HandlerFunc(w.Wrap(h.createHandler))
	router.Path(ServiceBindingsPath).Methods("GET").HandlerFunc(w.Wrap(h.listHandler))
	router.Path(ServiceBindingPath).Methods("DELETE").HandlerFunc(w.Wrap(h.deleteHandler))
}

// TODO: Separate commit/PR to move this function into shared.go and refactor all the handlers
// https://github.com/cloudfoundry/korifi/issues/698
func isUnknownKeyError(err error) bool {
	switch err.(type) {
	case schema.MultiError:
		multiError := err.(schema.MultiError)
		for _, v := range multiError {
			_, ok := v.(schema.UnknownKeyError)
			if ok {
				return true
			}
		}
	}
	return false
}
