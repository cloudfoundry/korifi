package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	SpacesPath                    = "/v3/spaces"
	SpacePath                     = "/v3/spaces/{guid}"
	RoutesForSpacePath            = "/v3/spaces/{guid}/routes"
	SpaceFeaturePath              = "/v3/spaces/{guid}/features/{name}"
	IsolationSegmentsForSpacePath = "/v3/spaces/{guid}/relationships/isolation_segment"
)

//counterfeiter:generate -o fake -fake-name CFSpaceRepository . CFSpaceRepository

type CFSpaceRepository interface {
	CreateSpace(context.Context, authorization.Info, repositories.CreateSpaceMessage) (repositories.SpaceRecord, error)
	ListSpaces(context.Context, authorization.Info, repositories.ListSpacesMessage) (repositories.ListResult[repositories.SpaceRecord], error)
	GetSpace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	DeleteSpace(context.Context, authorization.Info, repositories.DeleteSpaceMessage) error
	PatchSpaceMetadata(context.Context, authorization.Info, repositories.PatchSpaceMetadataMessage) (repositories.SpaceRecord, error)
	GetDeletedAt(context.Context, authorization.Info, string) (*time.Time, error)
}

type Space struct {
	spaceRepo        CFSpaceRepository
	routeRepo        CFRouteRepository
	orgRepo          CFOrgRepository
	apiBaseURL       url.URL
	requestValidator RequestValidator
	includeResolver  *include.IncludeResolver[
		[]repositories.SpaceRecord,
		repositories.SpaceRecord,
	]
}

func NewSpace(apiBaseURL url.URL, spaceRepo CFSpaceRepository, orgRepo CFOrgRepository, routeRepo CFRouteRepository, requestValidator RequestValidator, relationshipRepo include.ResourceRelationshipRepository) *Space {
	return &Space{
		apiBaseURL:       apiBaseURL,
		spaceRepo:        spaceRepo,
		orgRepo:          orgRepo,
		routeRepo:        routeRepo,
		requestValidator: requestValidator,
		includeResolver:  include.NewIncludeResolver[[]repositories.SpaceRecord](relationshipRepo, presenter.NewResource(apiBaseURL)),
	}
}

func (h *Space) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.create")

	var payload payloads.SpaceCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	payload := new(payloads.SpaceList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to decode and validate request values")
	}

	spaces, err := h.spaceRepo.ListSpaces(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch spaces")
	}

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, spaces.Records, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForSpace, spaces, h.apiBaseURL, *r.URL, includedResources...)), nil
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
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	payload := new(payloads.SpaceGet)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	space, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch space", "spaceGUID", spaceGUID)
	}

	includedResources, err := h.includeResolver.ResolveIncludes(r.Context(), authInfo, []repositories.SpaceRecord{space}, payload.IncludeResourceRules)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to build included resources")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForSpace(space, h.apiBaseURL, includedResources...)), nil
}

func (h *Space) deleteUnmappedRoutes(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.delete-unmapped-routes")

	spaceDeleteRoutes := new(payloads.SpaceDeleteRoutes)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, spaceDeleteRoutes); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to decode and validate request values")
	}

	spaceGUID := routing.URLParam(r, "guid")
	_, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch space", "spaceGUID", spaceGUID)
	}

	err = h.routeRepo.DeleteUnmappedRoutes(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete unmapped routes for space", "SpaceGUID", spaceGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(spaceGUID, presenter.SpaceDeleteUnmappedRoutesOperation, h.apiBaseURL)), nil
}

func (h *Space) getIsolationSegments(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForIsolationSegment(h.apiBaseURL)), nil
}

func (h *Space) getSpaceFeature(r *http.Request) (*routing.Response, error) {
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.space.get-feature")

	featureName := routing.URLParam(r, "name")
	if featureName != "ssh" {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewUnprocessableEntityError(nil, fmt.Sprintf("feature %q is not supported", featureName)), "feature not supported")
	}

	return routing.NewResponse(http.StatusOK).WithBody(map[string]any{
		"name":        "ssh",
		"description": "Enable SSHing into apps in the space.",
		"enabled":     false,
	}), nil
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
		{Method: "DELETE", Pattern: RoutesForSpacePath, Handler: h.deleteUnmappedRoutes},
		{Method: "GET", Pattern: IsolationSegmentsForSpacePath, Handler: h.getIsolationSegments},
		{Method: "GET", Pattern: SpaceFeaturePath, Handler: h.getSpaceFeature},
	}
}
