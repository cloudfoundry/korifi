package handlers

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
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
	PatchSpaceMetadata(context.Context, authorization.Info, repositories.PatchSpaceMetadataMessage) (repositories.SpaceRecord, error)
}

type Space struct {
	spaceRepo        SpaceRepository
	apiBaseURL       url.URL
	decoderValidator *DecoderValidator
}

func NewSpace(apiBaseURL url.URL, spaceRepo SpaceRepository, decoderValidator *DecoderValidator) *Space {
	return &Space{
		apiBaseURL:       apiBaseURL,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *Space) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.create")

	var payload payloads.SpaceCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to decode and validate payload")
	}

	space := payload.ToMessage()
	record, err := h.spaceRepo.CreateSpace(r.Context(), authInfo, space)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, "Invalid organization. Ensure the organization exists and you have access to it.", apierrors.NotFoundError{}),
			"Failed to create space",
			"Space Name", space.Name,
		)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForSpace(record, h.apiBaseURL)), nil
}

func (h *Space) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.list")

	orgUIDs := parseCommaSeparatedList(r.URL.Query().Get("organization_guids"))
	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	spaces, err := h.spaceRepo.ListSpaces(r.Context(), authInfo, repositories.ListSpacesMessage{
		OrganizationGUIDs: orgUIDs,
		Names:             names,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch spaces")
	}

	spaceList := presenter.ForList(presenter.ForSpace, spaces, h.apiBaseURL, *r.URL)
	return routing.NewResponse(http.StatusOK).WithBody(spaceList), nil
}

//nolint:dupl
func (h *Space) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.update")

	spaceGUID := routing.URLParam(r, "guid")

	space, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch org from Kubernetes", "GUID", spaceGUID)
	}

	var payload payloads.SpacePatch
	if err = h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	space, err = h.spaceRepo.PatchSpaceMetadata(r.Context(), authInfo, payload.ToMessage(spaceGUID, space.OrganizationGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch space metadata", "GUID", spaceGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForSpace(space, h.apiBaseURL)), nil
}

func (h *Space) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.delete")

	spaceGUID := routing.URLParam(r, "guid")

	spaceRecord, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch space", "SpaceGUID", spaceGUID)
	}

	deleteSpaceMessage := repositories.DeleteSpaceMessage{
		GUID:             spaceRecord.GUID,
		OrganizationGUID: spaceRecord.OrganizationGUID,
	}
	err = h.spaceRepo.DeleteSpace(r.Context(), authInfo, deleteSpaceMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete space", "SpaceGUID", spaceGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(spaceGUID, presenter.SpaceDeleteOperation, h.apiBaseURL)), nil
}

func (h *Space) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.get")

	spaceGUID := routing.URLParam(r, "guid")

	space, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch space from Kubernetes", "GUID", spaceGUID)
	}

	spaceResult := presenter.ForSpace(space, h.apiBaseURL)
	return routing.NewResponse(http.StatusOK).WithBody(spaceResult), nil
}

func (h *Space) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Space) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: SpacesPath, Handler: h.list},
		{Method: "POST", Pattern: SpacesPath, Handler: h.create},
		{Method: "PATCH", Pattern: SpacePath, Handler: h.update},
		{Method: "DELETE", Pattern: SpacePath, Handler: h.delete},
		{Method: "GET", Pattern: SpacePath, Handler: h.get},
	}
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
