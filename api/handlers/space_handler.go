package handlers

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	SpacesPath             = "/v3/spaces"
	SpacePath              = "/v3/spaces/{guid}"
	SpaceManifestApplyPath = "/v3/spaces/{spaceGUID}/actions/apply_manifest"
	SpaceManifestDiffPath  = "/v3/spaces/{spaceGUID}/manifest_diff"
)

//counterfeiter:generate -o fake -fake-name SpaceRepository . SpaceRepository

type SpaceRepository interface {
	CreateSpace(context.Context, authorization.Info, repositories.CreateSpaceMessage) (repositories.SpaceRecord, error)
	ListSpaces(context.Context, authorization.Info, repositories.ListSpacesMessage) ([]repositories.SpaceRecord, error)
	GetSpace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	DeleteSpace(context.Context, authorization.Info, repositories.DeleteSpaceMessage) error
	PatchSpaceMetadata(context.Context, authorization.Info, repositories.PatchSpaceMetadataMessage) (repositories.SpaceRecord, error)
}

//counterfeiter:generate -o fake -fake-name ManifestApplier . ManifestApplier

type ManifestApplier interface {
	Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, manifest payloads.Manifest) error
}

type SpaceHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	spaceRepo        SpaceRepository
	manifestApplier  ManifestApplier
	apiBaseURL       url.URL
	decoderValidator *DecoderValidator
}

func NewSpaceHandler(apiBaseURL url.URL, spaceRepo SpaceRepository, manifestApplier ManifestApplier, decoderValidator *DecoderValidator) *SpaceHandler {
	return &SpaceHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("SpaceHandler")),
		apiBaseURL:       apiBaseURL,
		spaceRepo:        spaceRepo,
		manifestApplier:  manifestApplier,
		decoderValidator: decoderValidator,
	}
}

func (h *SpaceHandler) spaceCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.SpaceCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to decode and validate payload")
	}

	space := payload.ToMessage()
	record, err := h.spaceRepo.CreateSpace(ctx, authInfo, space)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, "Invalid organization. Ensure the organization exists and you have access to it.", apierrors.NotFoundError{}),
			"Failed to create space",
			"Space Name", space.Name,
		)
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForSpace(record, h.apiBaseURL)), nil
}

func (h *SpaceHandler) spaceListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	orgUIDs := parseCommaSeparatedList(r.URL.Query().Get("organization_guids"))
	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	spaces, err := h.spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
		OrganizationGUIDs: orgUIDs,
		Names:             names,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch spaces")
	}

	spaceList := presenter.ForSpaceList(spaces, h.apiBaseURL, *r.URL)
	return NewHandlerResponse(http.StatusOK).WithBody(spaceList), nil
}

//nolint:dupl
func (h *SpaceHandler) spacePatchHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	spaceGUID := chi.URLParam(r, "guid")

	space, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch org from Kubernetes", "GUID", spaceGUID)
	}

	var payload payloads.SpacePatch
	if err = h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	space, err = h.spaceRepo.PatchSpaceMetadata(ctx, authInfo, payload.ToMessage(spaceGUID, space.OrganizationGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch space metadata", "GUID", spaceGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForSpace(space, h.apiBaseURL)), nil
}

func (h *SpaceHandler) spaceDeleteHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	spaceGUID := chi.URLParam(r, "guid")

	spaceRecord, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch space", "SpaceGUID", spaceGUID)
	}

	deleteSpaceMessage := repositories.DeleteSpaceMessage{
		GUID:             spaceRecord.GUID,
		OrganizationGUID: spaceRecord.OrganizationGUID,
	}
	err = h.spaceRepo.DeleteSpace(ctx, authInfo, deleteSpaceMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete space", "SpaceGUID", spaceGUID)
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(spaceGUID, presenter.SpaceDeleteOperation, h.apiBaseURL)), nil
}

func (h *SpaceHandler) applyManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	spaceGUID := chi.URLParam(r, "spaceGUID")
	var manifest payloads.Manifest
	if err := h.decoderValidator.DecodeAndValidateYAMLPayload(r, &manifest); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if err := h.manifestApplier.Apply(ctx, authInfo, spaceGUID, manifest); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error applying manifest")
	}

	return NewHandlerResponse(http.StatusAccepted).
		WithHeader(headers.Location, presenter.JobURLForRedirects(spaceGUID, presenter.SpaceApplyManifestOperation, h.apiBaseURL)), nil
}

func (h *SpaceHandler) diffManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	spaceGUID := chi.URLParam(r, "spaceGUID")

	if _, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get space", "guid", spaceGUID)
	}

	return NewHandlerResponse(http.StatusAccepted).WithBody(map[string]interface{}{"diff": []string{}}), nil
}

func (h *SpaceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router := chi.NewRouter()
	router.Get(SpacesPath, h.handlerWrapper.Wrap(h.spaceListHandler))
	router.Post(SpacesPath, h.handlerWrapper.Wrap(h.spaceCreateHandler))
	router.Patch(SpacePath, h.handlerWrapper.Wrap(h.spacePatchHandler))
	router.Delete(SpacePath, h.handlerWrapper.Wrap(h.spaceDeleteHandler))
	router.Post(SpaceManifestApplyPath, h.handlerWrapper.Wrap(h.applyManifestHandler))
	router.Post(SpaceManifestDiffPath, h.handlerWrapper.Wrap(h.diffManifestHandler))
	router.ServeHTTP(w, r)
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
