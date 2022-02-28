package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	SpaceManifestApplyEndpoint = "/v3/spaces/{spaceGUID}/actions/apply_manifest"
	SpaceManifestDiffEndpoint  = "/v3/spaces/{spaceGUID}/manifest_diff"
)

type SpaceManifestHandler struct {
	logger              logr.Logger
	serverURL           url.URL
	defaultDomainName   string
	applyManifestAction ApplyManifestAction
	spaceRepo           repositories.CFSpaceRepository
	decoderValidator    *DecoderValidator
}

//counterfeiter:generate -o fake -fake-name ApplyManifestAction . ApplyManifestAction
type ApplyManifestAction func(ctx context.Context, authInfo authorization.Info, spaceGUID string, defaultDomainName string, manifest payloads.Manifest) error

func NewSpaceManifestHandler(
	logger logr.Logger,
	serverURL url.URL,
	defaultDomainName string,
	applyManifestAction ApplyManifestAction,
	spaceRepo repositories.CFSpaceRepository,
	decoderValidator *DecoderValidator,
) *SpaceManifestHandler {
	return &SpaceManifestHandler{
		logger:              logger,
		serverURL:           serverURL,
		defaultDomainName:   defaultDomainName,
		applyManifestAction: applyManifestAction,
		spaceRepo:           spaceRepo,
		decoderValidator:    decoderValidator,
	}
}

func (h *SpaceManifestHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(SpaceManifestApplyEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.applyManifestHandler))
	router.Path(SpaceManifestDiffEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.validateSpaceVisible(h.diffManifestHandler)))
}

func (h *SpaceManifestHandler) applyManifestHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	spaceGUID := vars["spaceGUID"]
	var manifest payloads.Manifest
	rme := h.decoderValidator.DecodeAndValidateYAMLPayload(r, &manifest)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	err := h.applyManifestAction(r.Context(), authInfo, spaceGUID, h.defaultDomainName, manifest)
	if err != nil {
		h.handleApplyManifestErr(err, w, h.defaultDomainName)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("%s/v3/jobs/space.apply_manifest-%s", h.serverURL.String(), spaceGUID))
	w.WriteHeader(http.StatusAccepted)
}

func (h *SpaceManifestHandler) handleApplyManifestErr(err error, w http.ResponseWriter, defaultDomainName string) {
	switch err.(type) {
	case repositories.NotFoundError:
		h.logger.Info("Domain not found", "error: ", err.Error())
		writeUnprocessableEntityError(w, "The configured default domain `"+defaultDomainName+"` was not found")
	default:
		h.logger.Error(err, "Error applying manifest")
		writeUnknownErrorResponse(w)
	}
}

func (h *SpaceManifestHandler) diffManifestHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"diff":[]}`))
}

func (h *SpaceManifestHandler) validateSpaceVisible(hf AuthAwareHandlerFunc) AuthAwareHandlerFunc {
	return func(info authorization.Info, w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		spaceGUID := vars["spaceGUID"]
		w.Header().Set("Content-Type", "application/json")

		spaces, err := h.spaceRepo.ListSpaces(r.Context(), info, repositories.ListSpacesMessage{})
		if err != nil {
			h.logger.Error(err, "Failed to list spaces")
			writeUnknownErrorResponse(w)
			return
		}

		spaceNotFound := true
		for _, space := range spaces {
			if space.GUID == spaceGUID {
				spaceNotFound = false
				break
			}
		}

		if spaceNotFound {
			writeNotFoundErrorResponse(w, "Space")
			return
		}

		hf(info, w, r)
	}
}
