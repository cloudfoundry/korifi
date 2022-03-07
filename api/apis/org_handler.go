package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
)

const (
	OrgsEndpoint           = "/v3/organizations"
	OrgDeleteEndpoint      = "/v3/organizations/{guid}"
	OrgListDomainsEndpoint = "/v3/organizations/{guid}/domains"
)

//counterfeiter:generate -o fake -fake-name OrgRepository . CFOrgRepository
type CFOrgRepository interface {
	CreateOrg(context.Context, authorization.Info, repositories.CreateOrgMessage) (repositories.OrgRecord, error)
	ListOrgs(context.Context, authorization.Info, repositories.ListOrgsMessage) ([]repositories.OrgRecord, error)
	DeleteOrg(context.Context, authorization.Info, repositories.DeleteOrgMessage) error
	GetOrg(context.Context, authorization.Info, string) (repositories.OrgRecord, error)
}

type OrgHandler struct {
	logger           logr.Logger
	apiBaseURL       url.URL
	orgRepo          CFOrgRepository
	domainRepo       CFDomainRepository
	decoderValidator *DecoderValidator
}

func NewOrgHandler(apiBaseURL url.URL, orgRepo CFOrgRepository, domainRepo CFDomainRepository, decoderValidator *DecoderValidator) *OrgHandler {
	return &OrgHandler{
		logger:           controllerruntime.Log.WithName("Org Handler"),
		apiBaseURL:       apiBaseURL,
		orgRepo:          orgRepo,
		domainRepo:       domainRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *OrgHandler) orgCreateHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.OrgCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	org := payload.ToMessage()
	org.GUID = uuid.NewString()

	record, err := h.orgRepo.CreateOrg(r.Context(), info, org)
	if err != nil {
		if webhooks.HasErrorCode(err, webhooks.DuplicateOrgNameError) {
			errorDetail := fmt.Sprintf("Organization '%s' already exists.", org.Name)
			h.logger.Info(errorDetail)
			return nil, apierrors.NewUnprocessableEntityError(err, errorDetail)
		}

		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to create org")
			return nil, apierrors.NewInvalidAuthError(err)
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to create org")
			return nil, apierrors.NewNotAuthenticatedError(err)
		}

		if repositories.IsForbiddenError(err) {
			h.logger.Error(err, "not allowed to create orgs")
			return nil, apierrors.NewForbiddenError(err, repositories.OrgResourceType)
		}

		h.logger.Error(err, "Failed to create org", "Org Name", payload.Name)
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForCreateOrg(record, h.apiBaseURL)), nil
}

func (h *OrgHandler) orgDeleteHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	orgGUID := vars["guid"]

	deleteOrgMessage := repositories.DeleteOrgMessage{
		GUID: orgGUID,
	}
	err := h.orgRepo.DeleteOrg(ctx, info, deleteOrgMessage)
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Error(err, "unauthorized to delete org", "OrgGUID", orgGUID)
			return nil, apierrors.NewForbiddenError(err, repositories.OrgResourceType)
		case repositories.NotFoundError:
			h.logger.Info("Org not found", "OrgGUID", orgGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.OrgResourceType)
		default:
			h.logger.Error(err, "Failed to delete org", "OrgGUID", orgGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", fmt.Sprintf("%s/v3/jobs/org.delete-%s", h.apiBaseURL.String(), orgGUID)), nil
}

func (h *OrgHandler) orgListHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	orgs, err := h.orgRepo.ListOrgs(ctx, info, repositories.ListOrgsMessage{Names: names})
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to list orgs")
			return nil, apierrors.NewInvalidAuthError(err)
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to list orgs")
			return nil, apierrors.NewNotAuthenticatedError(err)
		}

		h.logger.Error(err, "failed to fetch orgs")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForOrgList(orgs, h.apiBaseURL, *r.URL)), nil
}

func (h *OrgHandler) orgListDomainHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl
	ctx := r.Context()

	vars := mux.Vars(r)
	orgGUID := vars["guid"]

	if _, err := h.orgRepo.GetOrg(ctx, info, orgGUID); err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Error(err, "Organization not found", "OrgGUID", orgGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.OrgResourceType)
		default:
			h.logger.Error(err, "Unable to get organization")
			return nil, err
		}
	}

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	domainListFilter := new(payloads.DomainList)
	err := schema.NewDecoder().Decode(domainListFilter, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Organization Domain filter")
					return nil, apierrors.NewUnknownKeyError(err, domainListFilter.SupportedFilterKeys())
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err

		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	domainList, err := h.domainRepo.ListDomains(ctx, info, domainListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch domain(s) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDomainList(domainList, h.apiBaseURL, *r.URL)), nil
}

func (h *OrgHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(OrgsEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.orgListHandler))
	router.Path(OrgDeleteEndpoint).Methods("DELETE").HandlerFunc(w.Wrap(h.orgDeleteHandler))
	router.Path(OrgsEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.orgCreateHandler))
	router.Path(OrgListDomainsEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.orgListDomainHandler))
}
