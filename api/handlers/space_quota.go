package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	SpaceQuotasPath      = "/v3/space_quotas"
	SpaceQuotaPath       = "/v3/space_quotas/{guid}"
	SpaceQuotaSpacesPath = "/v3/space_quotas/{guid}/relationships/spaces"
)

//counterfeiter:generate -o fake -fake-name CFOrgRepository . CFOrgRepository
type SpaceQuotaRepository interface {
	CreateSpaceQuota(context.Context, authorization.Info, korifiv1alpha1.SpaceQuota) (korifiv1alpha1.SpaceQuotaResource, error)
	ListSpaceQuotas(context.Context, authorization.Info, repositories.ListSpaceQuotasMessage) ([]korifiv1alpha1.SpaceQuotaResource, error)
	DeleteSpaceQuota(context.Context, authorization.Info, string) error
	GetSpaceQuota(context.Context, authorization.Info, string) (korifiv1alpha1.SpaceQuotaResource, error)
	PatchSpaceQuota(context.Context, authorization.Info, string, korifiv1alpha1.SpaceQuotaPatch) (korifiv1alpha1.SpaceQuotaResource, error)
	AddSpaceQuotaRelationships(context.Context, authorization.Info, string, korifiv1alpha1.ToManyRelationship) (korifiv1alpha1.ToManyRelationship, error)
}

type SpaceQuota struct {
	apiBaseURL       url.URL
	spaceQuotaRepo   SpaceQuotaRepository
	requestValidator RequestValidator
}

func NewSpaceQuota(apiBaseURL url.URL, spaceQuotaRepo SpaceQuotaRepository, requestValidator RequestValidator) *SpaceQuota {
	return &SpaceQuota{
		apiBaseURL:       apiBaseURL,
		spaceQuotaRepo:   spaceQuotaRepo,
		requestValidator: requestValidator,
	}
}

func (h *SpaceQuota) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.spacequota.create")

	var spaceQuota korifiv1alpha1.SpaceQuota
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &spaceQuota); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "invalid-payload-for-create-space-quota")
	}

	spaceQuotaResource, err := h.spaceQuotaRepo.CreateSpaceQuota(r.Context(), authInfo, spaceQuota)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create space quota", "SpaceQuota Name", spaceQuota.Name)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForSpaceQuota(spaceQuotaResource, h.apiBaseURL)), nil
}

func (h *SpaceQuota) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.spacequota.update")

	guid := routing.URLParam(r, "guid")

	_, err := h.spaceQuotaRepo.GetSpaceQuota(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to find space quota", "SpaceQuotaGUID", guid)
	}

	var spaceQuotaPatch korifiv1alpha1.SpaceQuotaPatch
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &spaceQuotaPatch); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	updatedSpaceQuota, err := h.spaceQuotaRepo.PatchSpaceQuota(r.Context(), authInfo, guid, spaceQuotaPatch)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch org quota", "SpaceQuotaGUID", guid)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForSpaceQuota(updatedSpaceQuota, h.apiBaseURL)), nil
}

func (h *SpaceQuota) addSpaces(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.update")
	guid := routing.URLParam(r, "guid")

	_, err := h.spaceQuotaRepo.GetSpaceQuota(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to find org quota", "OrgQuotaGUID", guid)
	}

	var toManyRelationships korifiv1alpha1.ToManyRelationship
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &toManyRelationships); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	actualRelationships, err := h.spaceQuotaRepo.AddSpaceQuotaRelationships(r.Context(), authInfo, guid, toManyRelationships)
	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForSpaceQuotaRelationships(guid, actualRelationships, h.apiBaseURL)), nil
}

func (h *SpaceQuota) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.delete")

	spaceQuotaGUID := routing.URLParam(r, "guid")

	err := h.spaceQuotaRepo.DeleteSpaceQuota(r.Context(), authInfo, spaceQuotaGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to delete space quota", "SpaceGUID", spaceQuotaGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(spaceQuotaGUID, presenter.SpaceQuotaDeleteOperation, h.apiBaseURL)), nil
}

func (h *SpaceQuota) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.spacequota.list")

	listFilter := &payloads.SpaceQuotaList{}
	err := h.requestValidator.DecodeAndValidateURLValues(r, listFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	spaceQuotas, err := h.spaceQuotaRepo.ListSpaceQuotas(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to fetch space quotas")
	}

	resp := routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForSpaceQuota, spaceQuotas, h.apiBaseURL, *r.URL))

	return resp, nil
}

func (h *SpaceQuota) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.get")

	spaceQuotaGUID := routing.URLParam(r, "guid")

	spaceQuota, err := h.spaceQuotaRepo.GetSpaceQuota(r.Context(), authInfo, spaceQuotaGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to get space quota", "SpaceQuotaGUID", spaceQuotaGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForSpaceQuota(spaceQuota, h.apiBaseURL)), nil
}

func (h *SpaceQuota) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *SpaceQuota) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: SpaceQuotasPath, Handler: h.list},
		{Method: "POST", Pattern: SpaceQuotasPath, Handler: h.create},
		{Method: "POST", Pattern: SpaceQuotaSpacesPath, Handler: h.addSpaces},
		{Method: "DELETE", Pattern: SpaceQuotaPath, Handler: h.delete},
		{Method: "PATCH", Pattern: SpaceQuotaPath, Handler: h.update},
		{Method: "GET", Pattern: SpaceQuotaPath, Handler: h.get},
	}
}
