package apis

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	DomainListEndpoint = "/v3/domains"
)

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	GetDomain(context.Context, authorization.Info, string) (repositories.DomainRecord, error)
	ListDomains(context.Context, authorization.Info, repositories.ListDomainsMessage) ([]repositories.DomainRecord, error)
}

type DomainHandler struct {
	logger     logr.Logger
	serverURL  url.URL
	domainRepo CFDomainRepository
}

func NewDomainHandler(
	logger logr.Logger,
	serverURL url.URL,
	domainRepo CFDomainRepository,
) *DomainHandler {
	return &DomainHandler{
		logger:     logger,
		serverURL:  serverURL,
		domainRepo: domainRepo,
	}
}

func (h *DomainHandler) DomainListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl
	ctx := r.Context()

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
					h.logger.Info("Unknown key used in Domain filter")
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

	domainList, err := h.domainRepo.ListDomains(ctx, authInfo, domainListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch domain(s) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDomainList(domainList, h.serverURL, *r.URL)), nil
}

func (h *DomainHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(DomainListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.DomainListHandler))
}
