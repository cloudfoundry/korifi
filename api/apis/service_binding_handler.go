package apis

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	ServiceBindingCreateEndpoint = "/v3/service_credential_bindings"
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
	ServiceBindingExists(ctx context.Context, info authorization.Info, spaceGUID, appGUID, serviceInsanceGUID string) (bool, error)
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

func (h *ServiceBindingHandler) createHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json") // TODO: move this into the writeJsonResponse

	var payload payloads.ServiceBindingCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		h.writeErrorResponse(w, err, "get apps")
		return
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, payload.Relationships.ServiceInstance.Data.GUID)
	if err != nil {
		h.writeErrorResponse(w, err, "get service instances")
		return
	}

	if app.SpaceGUID != serviceInstance.SpaceGUID {
		h.logger.Info("App and ServiceInstance in different spaces", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
		writeUnprocessableEntityError(w, "The service instance and the app are in different spaces")
		return
	}

	bindingExists, err := h.serviceBindingRepo.ServiceBindingExists(ctx, authInfo, app.SpaceGUID, app.GUID, serviceInstance.GUID)
	if err != nil {
		h.writeErrorResponse(w, err, "get service bindings")
		return
	}
	if bindingExists {
		h.logger.Info("ServiceBinding already exists for App and ServiceInstance", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
		writeUnprocessableEntityError(w, "The app is already bound to the service instance")
		return
	}

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		h.writeErrorResponse(w, err, "create service bindings")
		return
	}

	writeResponse(w, http.StatusCreated, presenter.ForServiceBinding(serviceBinding, h.serverURL))
}

func (h *ServiceBindingHandler) writeErrorResponse(w http.ResponseWriter, err error, message string) {
	if repositories.IsForbiddenError(err) {
		h.logger.Error(err, "not allowed to "+message)
		writeNotAuthorizedErrorResponse(w)
	} else if authorization.IsInvalidAuth(err) {
		h.logger.Error(err, "invalid auth")
		writeInvalidAuthErrorResponse(w)
	} else if authorization.IsNotAuthenticated(err) {
		h.logger.Error(err, "not authenticated")
		writeNotAuthenticatedErrorResponse(w)
	} else {
		h.logger.Error(err, "unexpected error")
		writeUnknownErrorResponse(w)
	}
}

func (h *ServiceBindingHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ServiceBindingCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.createHandler))
}
