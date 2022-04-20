package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	OrgsPath       = "/v3/organizations"
	OrgPath        = "/v3/organizations/{guid}"
	OrgDomainsPath = "/v3/organizations/{guid}/domains"
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
	record, err := h.orgRepo.CreateOrg(r.Context(), info, org)
	if err != nil {
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
		h.logger.Error(err, "Failed to delete org", "OrgGUID", orgGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", fmt.Sprintf("%s/v3/jobs/org.delete-%s", h.apiBaseURL.String(), orgGUID)), nil
}

func (h *OrgHandler) orgListHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	orgs, err := h.orgRepo.ListOrgs(ctx, info, repositories.ListOrgsMessage{Names: names})
	if err != nil {
		h.logger.Error(err, "failed to fetch orgs")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForOrgList(orgs, h.apiBaseURL, *r.URL)), nil
}

func (h *OrgHandler) orgListDomainHandler(info authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	orgGUID := vars["guid"]

	if _, err := h.orgRepo.GetOrg(ctx, info, orgGUID); err != nil {
		h.logger.Error(err, "Unable to get organization")
		return nil, err
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
	router.Path(OrgsPath).Methods("GET").HandlerFunc(w.Wrap(h.orgListHandler))
	router.Path(OrgPath).Methods("DELETE").HandlerFunc(w.Wrap(h.orgDeleteHandler))
	router.Path(OrgsPath).Methods("POST").HandlerFunc(w.Wrap(h.orgCreateHandler))
	router.Path(OrgDomainsPath).Methods("GET").HandlerFunc(w.Wrap(h.orgListDomainHandler))
}
