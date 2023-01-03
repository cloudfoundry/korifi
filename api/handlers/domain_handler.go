package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"

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

type DomainHandler struct {
	serverURL            url.URL
	requestJSONValidator RequestJSONValidator
	domainRepo           CFDomainRepository
}

func NewDomainHandler(
	serverURL url.URL,
	requestJSONValidator RequestJSONValidator,
	domainRepo CFDomainRepository,
) *DomainHandler {
	return &DomainHandler{
		serverURL:            serverURL,
		requestJSONValidator: requestJSONValidator,
		domainRepo:           domainRepo,
	}
}

func (h *DomainHandler) domainCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.DomainCreate
	if err := h.requestJSONValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	domainCreateMessage, err := payload.ToMessage()
	if err != nil {
		apierr := apierrors.NewUnprocessableEntityError(err, "Error converting domain payload to repository message: "+err.Error())
		return nil, apierrors.LogAndReturn(logger, apierr, apierr.Detail())
	}

	domain, err := h.domainRepo.CreateDomain(ctx, authInfo, domainCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error creating domain in repository")
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForDomain(domain, h.serverURL)), nil
}

func (h *DomainHandler) domainGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	domainGUID := URLParam(r, "guid")

	domain, err := h.domainRepo.GetDomain(ctx, authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting domain in repository")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDomain(domain, h.serverURL)), nil
}

func (h *DomainHandler) domainUpdateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	domainGUID := URLParam(r, "guid")

	var payload payloads.DomainUpdate
	if err := h.requestJSONValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	_, err := h.domainRepo.GetDomain(ctx, authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting domain in repository")
	}

	domain, err := h.domainRepo.UpdateDomain(ctx, authInfo, payload.ToMessage(domainGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating domain in repository")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDomain(domain, h.serverURL)), nil
}

func (h *DomainHandler) domainListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl
	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	domainListFilter := new(payloads.DomainList)
	err := payloads.Decode(domainListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	domainList, err := h.domainRepo.ListDomains(ctx, authInfo, domainListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch domain(s) from Kubernetes")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDomainList(domainList, h.serverURL, *r.URL)), nil
}

func (h *DomainHandler) domainDeleteHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	domainGUID := URLParam(r, "guid")

	err := h.domainRepo.DeleteDomain(ctx, authInfo, domainGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to delete domain from Kubernetes", "domainGUID", domainGUID)
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader(
		"Location",
		presenter.JobURLForRedirects(domainGUID, presenter.DomainDeleteOperation, h.serverURL),
	), nil
}

func (h *DomainHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}

func (h *DomainHandler) AuthenticatedRoutes() []Route {
	return []Route{
		{Method: "POST", Pattern: DomainsPath, HandlerFunc: h.domainCreateHandler},
		{Method: "GET", Pattern: DomainPath, HandlerFunc: h.domainGetHandler},
		{Method: "PATCH", Pattern: DomainPath, HandlerFunc: h.domainUpdateHandler},
		{Method: "GET", Pattern: DomainsPath, HandlerFunc: h.domainListHandler},
		{Method: "DELETE", Pattern: DomainPath, HandlerFunc: h.domainDeleteHandler},
	}
}
