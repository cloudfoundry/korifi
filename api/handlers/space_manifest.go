package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-chi/chi"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
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

type SpaceManifest struct {
	serverURL        url.URL
	manifestApplier  ManifestApplier
	spaceRepo        CFSpaceRepository
	decoderValidator *DecoderValidator
}

//counterfeiter:generate -o fake -fake-name ManifestApplier . ManifestApplier
type ManifestApplier interface {
	Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, manifest payloads.Manifest) error
}

func NewSpaceManifest(
	serverURL url.URL,
	manifestApplier ManifestApplier,
	spaceRepo CFSpaceRepository,
	decoderValidator *DecoderValidator,
) *SpaceManifest {
	return &SpaceManifest{
		serverURL:        serverURL,
		manifestApplier:  manifestApplier,
		spaceRepo:        spaceRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *SpaceManifest) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *SpaceManifest) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: SpaceManifestApplyPath, Handler: h.apply},
		{Method: "POST", Pattern: SpaceManifestDiffPath, Handler: h.diff},
	}
}

func (h *SpaceManifest) apply(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space-manifest.apply")

	spaceGUID := chi.URLParam(r, "spaceGUID")
	var manifest payloads.Manifest
	if err := h.decoderValidator.DecodeAndValidateYAMLPayload(r, &manifest); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if err := h.manifestApplier.Apply(r.Context(), authInfo, spaceGUID, manifest); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error applying manifest")
	}

	return routing.NewResponse(http.StatusAccepted).
		WithHeader(headers.Location, presenter.JobURLForRedirects(spaceGUID, presenter.SpaceApplyManifestOperation, h.serverURL)), nil
}

func (h *SpaceManifest) diff(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space-manifest.diff")

	spaceGUID := chi.URLParam(r, "spaceGUID")

	if _, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get space", "guid", spaceGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithBody(map[string]interface{}{"diff": []string{}}), nil
}
