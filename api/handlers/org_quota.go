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
	OrgQuotasPath             = "/v3/organization_quotas"
	OrgQuotaPath              = "/v3/organization_quotas/{guid}"
	OrgQuotaRelationshipsPath = "/v3/organization_quotas/{guid}/relationships/organizations"
)

//counterfeiter:generate -o fake -fake-name CFOrgRepository . CFOrgRepository
type CFOrgQuotaRepository interface {
	CreateOrgQuota(context.Context, authorization.Info, korifiv1alpha1.OrgQuota) (korifiv1alpha1.OrgQuotaResource, error)
	AddOrgQuotaRelationships(context.Context, authorization.Info, string, korifiv1alpha1.ToManyRelationship) (korifiv1alpha1.ToManyRelationship, error)
	ListOrgQuotas(context.Context, authorization.Info, repositories.ListOrgQuotasMessage) ([]korifiv1alpha1.OrgQuotaResource, error)
	DeleteOrgQuota(context.Context, authorization.Info, string) error
	GetOrgQuota(context.Context, authorization.Info, string) (korifiv1alpha1.OrgQuotaResource, error)
	PatchOrgQuota(context.Context, authorization.Info, string, korifiv1alpha1.OrgQuotaPatch) (korifiv1alpha1.OrgQuotaResource, error)
}

type OrgQuota struct {
	apiBaseURL       url.URL
	orgQuotaRepo     CFOrgQuotaRepository
	requestValidator RequestValidator
}

func NewOrgQuota(apiBaseURL url.URL, orgQuotaRepo CFOrgQuotaRepository, requestValidator RequestValidator) *OrgQuota {
	return &OrgQuota{
		apiBaseURL:       apiBaseURL,
		orgQuotaRepo:     orgQuotaRepo,
		requestValidator: requestValidator,
	}
}

func (h *OrgQuota) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.create")

	var orgQuota korifiv1alpha1.OrgQuota
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &orgQuota); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "invalid-payload-for-create-org-quota")
	}

	orgQuotaResource, err := h.orgQuotaRepo.CreateOrgQuota(r.Context(), authInfo, orgQuota)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create org", "OrgQuota Name", orgQuota.Name)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForOrgQuota(orgQuotaResource, h.apiBaseURL)), nil
}

func (h *OrgQuota) addRelationships(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.update")
	guid := routing.URLParam(r, "guid")

	_, err := h.orgQuotaRepo.GetOrgQuota(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to find org quota", "OrgQuotaGUID", guid)
	}

	var toManyRelationships korifiv1alpha1.ToManyRelationship
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &toManyRelationships); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	actualRelationships, err := h.orgQuotaRepo.AddOrgQuotaRelationships(r.Context(), authInfo, guid, toManyRelationships)
	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForOrgQuotaRelationships(guid, actualRelationships, h.apiBaseURL)), nil
}

func (h *OrgQuota) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.update")

	guid := routing.URLParam(r, "guid")

	_, err := h.orgQuotaRepo.GetOrgQuota(r.Context(), authInfo, guid)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to find org quota", "OrgQuotaGUID", guid)
	}

	var orgQuotaPatch korifiv1alpha1.OrgQuotaPatch
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &orgQuotaPatch); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	updatedOrgQuota, err := h.orgQuotaRepo.PatchOrgQuota(r.Context(), authInfo, guid, orgQuotaPatch)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch org metadata", "OrgQuotaGUID", guid)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForOrgQuota(updatedOrgQuota, h.apiBaseURL)), nil
}

func (h *OrgQuota) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.delete")

	orgQuotaGUID := routing.URLParam(r, "guid")

	err := h.orgQuotaRepo.DeleteOrgQuota(r.Context(), authInfo, orgQuotaGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to delete org", "OrgGUID", orgQuotaGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(orgQuotaGUID, presenter.OrgQuotaDeleteOperation, h.apiBaseURL)), nil
}

func (h *OrgQuota) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.list")

	listFilter := &payloads.OrgQuotaList{}
	err := h.requestValidator.DecodeAndValidateURLValues(r, listFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	orgQuotas, err := h.orgQuotaRepo.ListOrgQuotas(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to fetch orgs")
	}

	resp := routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForOrgQuota, orgQuotas, h.apiBaseURL, *r.URL))

	return resp, nil
}

func (h *OrgQuota) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.orgquota.get")

	logger.Info("bla", r.URL)
	orgQuotaGUID := routing.URLParam(r, "guid")

	orgQuota, err := h.orgQuotaRepo.GetOrgQuota(r.Context(), authInfo, orgQuotaGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to get org", "OrgQuotaGUID", orgQuotaGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForOrgQuota(orgQuota, h.apiBaseURL)), nil
}

func (h *OrgQuota) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *OrgQuota) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: OrgQuotasPath, Handler: h.list},
		{Method: "POST", Pattern: OrgQuotasPath, Handler: h.create},
		{Method: "POST", Pattern: OrgQuotaRelationshipsPath, Handler: h.addRelationships},
		{Method: "DELETE", Pattern: OrgQuotaPath, Handler: h.delete},
		{Method: "PATCH", Pattern: OrgQuotaPath, Handler: h.update},
		{Method: "GET", Pattern: OrgQuotaPath, Handler: h.get},
	}
}
