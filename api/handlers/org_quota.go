package handlers

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	OrgQuotasPath = "/v3/organization_quotas"
)

type OrgQuota struct {
	apiBaseURL url.URL
}

func NewOrgQuota(apiBaseURL url.URL) *OrgQuota {
	return &OrgQuota{
		apiBaseURL: apiBaseURL,
	}
}

func (h *OrgQuota) list(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.Empty, repositories.ListResult[any]{}, h.apiBaseURL, *r.URL)), nil
}

func (h *OrgQuota) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: OrgQuotasPath, Handler: h.list},
	}
}

func (h *OrgQuota) UnauthenticatedRoutes() []routing.Route {
	return nil
}
