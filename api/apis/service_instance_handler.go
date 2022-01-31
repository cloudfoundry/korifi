package apis

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gorilla/schema"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"

	"github.com/gorilla/mux"

	"github.com/go-logr/logr"
)

const (
	ServiceInstanceCreateEndpoint = "/v3/service_instances"
	ServiceInstanceListEndpoint   = "/v3/service_instances"
)

//counterfeiter:generate -o fake -fake-name CFServiceInstanceRepository . CFServiceInstanceRepository
type CFServiceInstanceRepository interface {
	CreateServiceInstance(context.Context, authorization.Info, repositories.CreateServiceInstanceMessage) (repositories.ServiceInstanceRecord, error)
	ListServiceInstances(context.Context, authorization.Info, repositories.ListServiceInstanceMessage) ([]repositories.ServiceInstanceRecord, error)
}

type ServiceInstanceHandler struct {
	logger              logr.Logger
	serverURL           url.URL
	serviceInstanceRepo CFServiceInstanceRepository
	appRepo             CFAppRepository
	decoderValidator    *DecoderValidator
}

func NewServiceInstanceHandler(
	logger logr.Logger,
	serverURL url.URL,
	serviceInstanceRepo CFServiceInstanceRepository,
	appRepo CFAppRepository,
	decoderValidator *DecoderValidator,
) *ServiceInstanceHandler {
	return &ServiceInstanceHandler{
		logger:              logger,
		serverURL:           serverURL,
		serviceInstanceRepo: serviceInstanceRepo,
		appRepo:             appRepo,
		decoderValidator:    decoderValidator,
	}
}

func (h *ServiceInstanceHandler) serviceInstanceCreateHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.ServiceInstanceCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	namespaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.appRepo.GetNamespace(ctx, authInfo, namespaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.PermissionDeniedOrNotFoundError:
			h.logger.Info("Namespace not found", "Namespace GUID", namespaceGUID)
			writeUnprocessableEntityError(w, "Invalid space. Ensure that the space exists and you have access to it.")
			return
		default:
			h.logger.Error(err, "Failed to fetch namespace from Kubernetes", "Namespace GUID", namespaceGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	serviceInstanceRecord, err := h.serviceInstanceRepo.CreateServiceInstance(ctx, authInfo, payload.ToServiceInstanceCreateMessage())
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to create service instance")
			writeInvalidAuthErrorResponse(w)

			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to create service instance")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		if repositories.IsForbiddenError(err) {
			h.logger.Error(err, "not allowed to create service instance")
			writeNotAuthorizedErrorResponse(w)

			return
		}

		h.logger.Error(err, "Failed to create service instance", "Service Instance Name", serviceInstanceRecord.Name)
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForServiceInstance(serviceInstanceRecord, h.serverURL), http.StatusCreated)
	if err != nil {
		// untested
		h.logger.Error(err, "Failed to render response", "ServiceInstance Name", payload.Name)
		writeUnknownErrorResponse(w)
	}
}

func (h *ServiceInstanceHandler) serviceInstanceListHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
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
					writeUnknownKeyError(w, listFilter.SupportedFilterKeys())
					return
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
		}
		return
	}

	serviceInstanceList, err := h.serviceInstanceRepo.ListServiceInstances(ctx, authInfo, listFilter.ToMessage())
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to list service instance")
			writeInvalidAuthErrorResponse(w)
			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to list service instance")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		if repositories.IsForbiddenError(err) {
			h.logger.Error(err, "not allowed to list service instance")
			writeNotAuthorizedErrorResponse(w)

			return
		}

		h.logger.Error(err, "Failed to list service instance")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForServiceInstanceList(serviceInstanceList, h.serverURL, *r.URL), http.StatusOK)
	if err != nil {
		// untested
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
}

func (h *ServiceInstanceHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ServiceInstanceCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.serviceInstanceCreateHandler))
	router.Path(ServiceInstanceListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.serviceInstanceListHandler))
}
