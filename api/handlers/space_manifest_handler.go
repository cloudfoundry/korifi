package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"

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
	handlerWrapper    *AuthAwareHandlerFuncWrapper
	serverURL         url.URL
	defaultDomainName string
	manifestApplier   ManifestApplier
	spaceRepo         CFSpaceRepository
	decoderValidator  *DecoderValidator
}

//counterfeiter:generate -o fake -fake-name ManifestApplier . ManifestApplier
type ManifestApplier interface {
	Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, defaultDomainName string, manifest payloads.Manifest) error
}

func NewSpaceManifestHandler(
	serverURL url.URL,
	defaultDomainName string,
	manifestApplier ManifestApplier,
	spaceRepo CFSpaceRepository,
	decoderValidator *DecoderValidator,
) *SpaceManifestHandler {
	return &SpaceManifestHandler{
		handlerWrapper:    NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("SpaceManifestHandler")),
		serverURL:         serverURL,
		defaultDomainName: defaultDomainName,
		manifestApplier:   manifestApplier,
		spaceRepo:         spaceRepo,
		decoderValidator:  decoderValidator,
	}
}

func (h *SpaceManifestHandler) RegisterRoutes(router *mux.Router) {
	router.Path(SpaceManifestApplyPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.applyManifestHandler))
	router.Path(SpaceManifestDiffPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.diffManifestHandler))
}

func (h *SpaceManifestHandler) applyManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	spaceGUID := vars["spaceGUID"]
	var manifest payloads.Manifest
	if err := h.decoderValidator.DecodeAndValidateYAMLPayload(r, &manifest); err != nil {
		return nil, err
	}

	if err := h.manifestApplier.Apply(ctx, authInfo, spaceGUID, h.defaultDomainName, manifest); err != nil {
		logger.Error(err, "Error applying manifest")
		return nil, err
	}

	return NewHandlerResponse(http.StatusAccepted).
		WithHeader(headers.Location, presenter.JobURLForRedirects(spaceGUID, presenter.SpaceApplyManifestOperation, h.serverURL)), nil
}

func (h *SpaceManifestHandler) diffManifestHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	spaceGUID := vars["spaceGUID"]

	if _, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID); err != nil {
		logger.Error(err, "failed to get space", "guid", spaceGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusAccepted).WithBody(map[string]interface{}{"diff": []string{}}), nil
}
