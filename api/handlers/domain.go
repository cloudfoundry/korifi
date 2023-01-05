package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	DomainsPath = "/v3/domains"
	DomainPath  = "/v3/domains/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	GetDomain(context.Context, authorization.Info, string) (repositories.DomainRecord, error)
	CreateDomain(context.Context, authorization.Info, repositories.CreateDomainMessage) (repositories.DomainRecord, error)
	UpdateDomain(context.Context, authorization.Info, repositories.UpdateDomainMessage) (repositories.DomainRecord, error)
	ListDomains(context.Context, authorization.Info, repositories.ListDomainsMessage) ([]repositories.DomainRecord, error)
	DeleteDomain(context.Context, authorization.Info, string) error
}

type Domain struct {
	serverURL            url.URL
	requestJSONValidator RequestJSONValidator
	domainRepo           CFDomainRepository
}

func NewDomain(
	serverURL url.URL,
	requestJSONValidator RequestJSONValidator,
	domainRepo CFDomainRepository,
) *Domain {
	return &Domain{
		serverURL:            serverURL,
		requestJSONValidator: requestJSONValidator,
		domainRepo:           domainRepo,
	}
}

func (h *Domain) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.create")

	var payload payloads.DomainCreate
	if err := h.requestJSONValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	domainCreateMessage, err := payload.ToMessage()
	if err != nil {
		apierr := apierrors.NewUnprocessableEntityError(err, "Error converting domain payload to repository message: "+err.Error())
		return nil, apierrors.LogAndReturn(logger, apierr, apierr.Detail())
	}

	domain, err := h.domainRepo.CreateDomain(r.Context(), authInfo, domainCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error creating domain in repository")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForDomain(domain, h.serverURL)), nil
}

func (h *Domain) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.get")

	domainGUID := routing.URLParam(r, "guid")

	domain, err := h.domainRepo.GetDomain(r.Context(), authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting domain in repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDomain(domain, h.serverURL)), nil
}

func (h *Domain) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.update")

	domainGUID := routing.URLParam(r, "guid")

	var payload payloads.DomainUpdate
	if err := h.requestJSONValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	_, err := h.domainRepo.GetDomain(r.Context(), authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting domain in repository")
	}

	domain, err := h.domainRepo.UpdateDomain(r.Context(), authInfo, payload.ToMessage(domainGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating domain in repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDomain(domain, h.serverURL)), nil
}

func (h *Domain) list(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	domainListFilter := new(payloads.DomainList)
	err := payloads.Decode(domainListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	domainList, err := h.domainRepo.ListDomains(r.Context(), authInfo, domainListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch domain(s) from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDomainList(domainList, h.serverURL, *r.URL)), nil
}

func (h *Domain) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.delete")

	domainGUID := routing.URLParam(r, "guid")

	err := h.domainRepo.DeleteDomain(r.Context(), authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to delete domain from Kubernetes", "domainGUID", domainGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader(
		"Location",
		presenter.JobURLForRedirects(domainGUID, presenter.DomainDeleteOperation, h.serverURL),
	), nil
}

func (h *Domain) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Domain) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: DomainsPath, Handler: h.create},
		{Method: "GET", Pattern: DomainPath, Handler: h.get},
		{Method: "PATCH", Pattern: DomainPath, Handler: h.update},
		{Method: "GET", Pattern: DomainsPath, Handler: h.list},
		{Method: "DELETE", Pattern: DomainPath, Handler: h.delete},
	}
}
