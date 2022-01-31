package apis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
)

const (
	SpaceCreateEndpoint = "/v3/spaces"
	SpaceDeleteEnpoint  = "/v3/spaces/{guid}"
	SpaceListEndpoint   = "/v3/spaces"
)

//counterfeiter:generate -o fake -fake-name SpaceRepository . SpaceRepository

type SpaceRepository interface {
	CreateSpace(context.Context, authorization.Info, repositories.CreateSpaceMessage) (repositories.SpaceRecord, error)
	ListSpaces(context.Context, authorization.Info, repositories.ListSpacesMessage) ([]repositories.SpaceRecord, error)
	GetSpace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	DeleteSpace(context.Context, authorization.Info, repositories.DeleteSpaceMessage) error
}

type SpaceHandler struct {
	spaceRepo               SpaceRepository
	logger                  logr.Logger
	apiBaseURL              url.URL
	imageRegistrySecretName string
	decoderValidator        *DecoderValidator
}

func NewSpaceHandler(apiBaseURL url.URL, imageRegistrySecretName string, spaceRepo SpaceRepository, decoderValidator *DecoderValidator) *SpaceHandler {
	return &SpaceHandler{
		apiBaseURL:              apiBaseURL,
		imageRegistrySecretName: imageRegistrySecretName,
		spaceRepo:               spaceRepo,
		logger:                  controllerruntime.Log.WithName("Space Handler"),
		decoderValidator:        decoderValidator,
	}
}

func (h *SpaceHandler) SpaceCreateHandler(info authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	var payload payloads.SpaceCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		h.logger.Error(rme, "Failed to decode and validate payload")
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	space := payload.ToMessage(h.imageRegistrySecretName)
	// TODO: Move this GUID generation down to the repository layer?
	space.GUID = uuid.NewString()

	record, err := h.spaceRepo.CreateSpace(ctx, info, space)
	if err != nil {
		if workloads.HasErrorCode(err, workloads.DuplicateSpaceNameError) {
			errorDetail := fmt.Sprintf("Space '%s' already exists.", space.Name)
			h.logger.Info(errorDetail)
			writeUnprocessableEntityError(w, errorDetail)
			return
		}

		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to create spaces")
			writeInvalidAuthErrorResponse(w)

			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to create spaces")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		if repositories.IsForbiddenError(err) {
			h.logger.Error(err, "not allowed to create spaces")
			writeNotAuthorizedErrorResponse(w)

			return
		}

		if errors.As(err, &repositories.PermissionDeniedOrNotFoundError{}) {
			h.logger.Error(err, "org does not exist or forbidden")
			writeUnprocessableEntityError(w, "Invalid organization. Ensure the organization exists and you have access to it.")
			return
		}

		h.logger.Error(err, "Failed to create space", "Space Name", space.Name)
		writeUnknownErrorResponse(w)
		return
	}

	spaceResponse := presenter.ForCreateSpace(record, h.apiBaseURL)
	writeResponse(w, http.StatusCreated, spaceResponse)
}

func (h *SpaceHandler) SpaceListHandler(info authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	orgUIDs := parseCommaSeparatedList(r.URL.Query().Get("organization_guids"))
	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	spaces, err := h.spaceRepo.ListSpaces(ctx, info, repositories.ListSpacesMessage{
		OrganizationGUIDs: orgUIDs,
		Names:             names,
	})
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to list spaces")
			writeInvalidAuthErrorResponse(w)

			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to list spaces")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		h.logger.Error(err, "Failed to fetch spaces")
		writeUnknownErrorResponse(w)

		return
	}

	spaceList := presenter.ForSpaceList(spaces, h.apiBaseURL, *r.URL)
	writeResponse(w, http.StatusOK, spaceList)
}

func (h *SpaceHandler) spaceDeleteHandler(info authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	spaceGUID := vars["guid"]

	spaceRecord, err := h.spaceRepo.GetSpace(ctx, info, spaceGUID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")

		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to get spaces")
			writeInvalidAuthErrorResponse(w)

			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to get spaces")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Space not found", "SpaceGUID", spaceGUID)
			writeNotFoundErrorResponse(w, "Space")
			return
		default:
			h.logger.Error(err, "Failed to fetch space", "SpaceGUID", spaceGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	deleteSpaceMessage := repositories.DeleteSpaceMessage{
		GUID:             spaceRecord.GUID,
		OrganizationGUID: spaceRecord.OrganizationGUID,
	}
	err = h.spaceRepo.DeleteSpace(ctx, info, deleteSpaceMessage)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")

		switch err.(type) {
		case authorization.InvalidAuthError:
			h.logger.Error(err, "unauthorized to delete spaces")
			writeNotAuthorizedErrorResponse(w)
			return
		default:
			h.logger.Error(err, "Failed to delete space", "SpaceGUID", spaceGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	w.Header().Set("Location", fmt.Sprintf("%s/v3/jobs/space.delete-%s", h.apiBaseURL.String(), spaceGUID))
	writeResponse(w, http.StatusAccepted, "")
}

func (h *SpaceHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(SpaceListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.SpaceListHandler))
	router.Path(SpaceCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.SpaceCreateHandler))
	router.Path(SpaceDeleteEnpoint).Methods("DELETE").HandlerFunc(w.Wrap(h.spaceDeleteHandler))
}

func parseCommaSeparatedList(list string) []string {
	var elements []string
	for _, element := range strings.Split(list, ",") {
		if element != "" {
			elements = append(elements, element)
		}
	}

	return elements
}
