package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	DomainsPath = "/v3/domains"
)

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	GetDomain(context.Context, authorization.Info, string) (repositories.DomainRecord, error)
	ListDomains(context.Context, authorization.Info, repositories.ListDomainsMessage) ([]repositories.DomainRecord, error)
}

type DomainHandler struct {
	handlerWrapper *AuthAwareHandlerFuncWrapper
	serverURL      url.URL
	domainRepo     CFDomainRepository
}

func NewDomainHandler(
	serverURL url.URL,
	domainRepo CFDomainRepository,
) *DomainHandler {
	return &DomainHandler{
		handlerWrapper: NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("DomainHandler")),
		serverURL:      serverURL,
		domainRepo:     domainRepo,
	}
}

func (h *DomainHandler) DomainListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl
	if err := r.ParseForm(); err != nil {
		logger.Error(err, "Unable to parse request query parameters")
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
					logger.Info("Unknown key used in Domain filter")
					return nil, apierrors.NewUnknownKeyError(err, domainListFilter.SupportedFilterKeys())
				}
			}
			logger.Error(err, "Unable to decode request query parameters")
			return nil, err

		default:
			logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	domainList, err := h.domainRepo.ListDomains(ctx, authInfo, domainListFilter.ToMessage())
	if err != nil {
		logger.Error(err, "Failed to fetch domain(s) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDomainList(domainList, h.serverURL, *r.URL)), nil
}

func (h *DomainHandler) RegisterRoutes(router *mux.Router) {
	router.Path(DomainsPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.DomainListHandler))
}
