package apis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
)

const (
	SpacesPath = "/v3/spaces"
	SpacePath  = "/v3/spaces/{guid}"
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

func (h *SpaceHandler) spaceCreateHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	var payload payloads.SpaceCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		h.logger.Error(err, "Failed to decode and validate payload")
		return nil, err
	}

	space := payload.ToMessage(h.imageRegistrySecretName)
	// TODO: Move this GUID generation down to the repository layer?
	space.GUID = uuid.NewString()

	record, err := h.spaceRepo.CreateSpace(ctx, info, space)
	if err != nil {
		if webhooks.HasErrorCode(err, webhooks.DuplicateSpaceNameError) {
			errorDetail := fmt.Sprintf("Space '%s' already exists.", space.Name)
			h.logger.Info(errorDetail)
			return nil, apierrors.NewUnprocessableEntityError(err, errorDetail)
		}

		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to create spaces")
			return nil, apierrors.NewInvalidAuthError(err)
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to create spaces")
			return nil, apierrors.NewNotAuthenticatedError(err)
		}

		if repositories.IsForbiddenError(err) {
			h.logger.Error(err, "not allowed to create spaces")
			return nil, apierrors.NewForbiddenError(err, repositories.SpaceResourceType)
		}

		if errors.As(err, &repositories.NotFoundError{}) {
			h.logger.Error(err, "org does not exist or forbidden")
			return nil, apierrors.NewUnprocessableEntityError(err, "Invalid organization. Ensure the organization exists and you have access to it.")
		}

		h.logger.Error(err, "Failed to create space", "Space Name", space.Name)
		return nil, err
	}

	spaceResponse := presenter.ForCreateSpace(record, h.apiBaseURL)
	return NewHandlerResponse(http.StatusCreated).WithBody(spaceResponse), nil
}

func (h *SpaceHandler) spaceListHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	orgUIDs := parseCommaSeparatedList(r.URL.Query().Get("organization_guids"))
	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	spaces, err := h.spaceRepo.ListSpaces(ctx, info, repositories.ListSpacesMessage{
		OrganizationGUIDs: orgUIDs,
		Names:             names,
	})
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to list spaces")
			return nil, apierrors.NewInvalidAuthError(err)
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to list spaces")
			return nil, apierrors.NewNotAuthenticatedError(err)
		}

		h.logger.Error(err, "Failed to fetch spaces")
		return nil, apierrors.NewUnknownError(err)
	}

	spaceList := presenter.ForSpaceList(spaces, h.apiBaseURL, *r.URL)
	return NewHandlerResponse(http.StatusOK).WithBody(spaceList), nil
}

func (h *SpaceHandler) spaceDeleteHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	spaceGUID := vars["guid"]

	spaceRecord, err := h.spaceRepo.GetSpace(ctx, info, spaceGUID)
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to get spaces")
			return nil, apierrors.NewInvalidAuthError(err)
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to get spaces")
			return nil, apierrors.NewNotAuthenticatedError(err)
		}

		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Space not found", "SpaceGUID", spaceGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.SpaceResourceType)
		default:
			h.logger.Error(err, "Failed to fetch space", "SpaceGUID", spaceGUID)
			return nil, apierrors.NewUnknownError(err)
		}
	}

	deleteSpaceMessage := repositories.DeleteSpaceMessage{
		GUID:             spaceRecord.GUID,
		OrganizationGUID: spaceRecord.OrganizationGUID,
	}
	err = h.spaceRepo.DeleteSpace(ctx, info, deleteSpaceMessage)
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Error(err, "unauthorized to delete spaces")
			return nil, apierrors.NewForbiddenError(err, repositories.SpaceResourceType)
		default:
			h.logger.Error(err, "Failed to delete space", "SpaceGUID", spaceGUID)
			return nil, apierrors.NewUnknownError(err)
		}
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader(headers.Location, fmt.Sprintf("%s/v3/jobs/space.delete-%s", h.apiBaseURL.String(), spaceGUID)), nil
}

func (h *SpaceHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(SpacesPath).Methods("GET").HandlerFunc(w.Wrap(h.spaceListHandler))
	router.Path(SpacesPath).Methods("POST").HandlerFunc(w.Wrap(h.spaceCreateHandler))
	router.Path(SpacePath).Methods("DELETE").HandlerFunc(w.Wrap(h.spaceDeleteHandler))
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
