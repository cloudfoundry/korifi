package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
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
	record, err := h.spaceRepo.CreateSpace(ctx, info, space)
	if err != nil {
		h.logger.Error(err, "Failed to create space", "Space Name", space.Name)
		return nil, apierrors.AsUnprocessibleEntity(err, "Invalid organization. Ensure the organization exists and you have access to it.", apierrors.NotFoundError{})
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
		h.logger.Error(err, "Failed to fetch spaces")
		return nil, err
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
		h.logger.Error(err, "Failed to fetch space", "SpaceGUID", spaceGUID)
		return nil, err
	}

	deleteSpaceMessage := repositories.DeleteSpaceMessage{
		GUID:             spaceRecord.GUID,
		OrganizationGUID: spaceRecord.OrganizationGUID,
	}
	err = h.spaceRepo.DeleteSpace(ctx, info, deleteSpaceMessage)
	if err != nil {
		h.logger.Error(err, "Failed to delete space", "SpaceGUID", spaceGUID)
		return nil, err
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
