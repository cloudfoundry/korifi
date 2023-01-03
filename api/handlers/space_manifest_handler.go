package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	SpaceManifestApplyPath = "/v3/spaces/{spaceGUID}/actions/apply_manifest"
	SpaceManifestDiffPath  = "/v3/spaces/{spaceGUID}/manifest_diff"
)

//counterfeiter:generate -o fake -fake-name CFSpaceRepository . CFSpaceRepository

type CFSpaceRepository interface {
	CreateSpace(context.Context, authorization.Info, repositories.CreateSpaceMessage) (repositories.SpaceRecord, error)
	ListSpaces(context.Context, authorization.Info, repositories.ListSpacesMessage) ([]repositories.SpaceRecord, error)
	GetSpace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	DeleteSpace(context.Context, authorization.Info, repositories.DeleteSpaceMessage) error
}

type SpaceManifestHandler struct {
	serverURL        url.URL
	manifestApplier  ManifestApplier
	spaceRepo        CFSpaceRepository
	decoderValidator *DecoderValidator
}

//counterfeiter:generate -o fake -fake-name ManifestApplier . ManifestApplier
type ManifestApplier interface {
	Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, manifest payloads.Manifest) error
}

func NewSpaceManifestHandler(
	serverURL url.URL,
	manifestApplier ManifestApplier,
	spaceRepo CFSpaceRepository,
	decoderValidator *DecoderValidator,
) *SpaceManifestHandler {
	return &SpaceManifestHandler{
		serverURL:        serverURL,
		manifestApplier:  manifestApplier,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *SpaceManifestHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}

func (h *SpaceManifestHandler) AuthenticatedRoutes() []AuthRoute {
	return []AuthRoute{
		{Method: "POST", Pattern: SpaceManifestApplyPath, HandlerFunc: h.applyManifestHandler},
		{Method: "POST", Pattern: SpaceManifestDiffPath, HandlerFunc: h.diffManifestHandler},
	}
}

func (h *SpaceManifestHandler) applyManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	spaceGUID := URLParam(r, "spaceGUID")
	var manifest payloads.Manifest
	if err := h.decoderValidator.DecodeAndValidateYAMLPayload(r, &manifest); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if err := h.manifestApplier.Apply(ctx, authInfo, spaceGUID, manifest); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error applying manifest")
	}

	return NewHandlerResponse(http.StatusAccepted).
		WithHeader(headers.Location, presenter.JobURLForRedirects(spaceGUID, presenter.SpaceApplyManifestOperation, h.serverURL)), nil
}

func (h *SpaceManifestHandler) diffManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	spaceGUID := URLParam(r, "spaceGUID")

	if _, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get space", "guid", spaceGUID)
	}

	return NewHandlerResponse(http.StatusAccepted).WithBody(map[string]interface{}{"diff": []string{}}), nil
}
