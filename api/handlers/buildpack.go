package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	BuildpacksPath = "/v3/buildpacks"
)

//counterfeiter:generate -o fake -fake-name BuildpackRepository . BuildpackRepository
type BuildpackRepository interface {
	ListBuildpacks(ctx context.Context, authInfo authorization.Info) ([]repositories.BuildpackRecord, error)
}

type Buildpack struct {
	serverURL        url.URL
	buildpackRepo    BuildpackRepository
	requestValidator RequestValidator
}

func NewBuildpack(
	serverURL url.URL,
	buildpackRepo BuildpackRepository,
	requestValidator RequestValidator,
) *Buildpack {
	return &Buildpack{
		serverURL:        serverURL,
		buildpackRepo:    buildpackRepo,
		requestValidator: requestValidator,
	}
}

func (h *Buildpack) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.build.list")

	buildpackListFilter := new(payloads.BuildpackList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, buildpackListFilter); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	buildpacks, err := h.buildpackRepo.ListBuildpacks(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch buildpacks from Kubernetes")
	}

	if err := h.sortList(buildpacks, buildpackListFilter.OrderBy); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "unable to parse order by request")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForBuildpack, buildpacks, h.serverURL, *r.URL)), nil
}

// nolint:dupl
func (h *Buildpack) sortList(bpList []repositories.BuildpackRecord, order string) error {
	switch order {
	case "":
	case "created_at":
		sort.Slice(bpList, func(i, j int) bool { return bpList[i].CreatedAt < bpList[j].CreatedAt })
	case "-created_at":
		sort.Slice(bpList, func(i, j int) bool { return bpList[i].CreatedAt > bpList[j].CreatedAt })
	case "updated_at":
		sort.Slice(bpList, func(i, j int) bool { return bpList[i].UpdatedAt < bpList[j].UpdatedAt })
	case "-updated_at":
		sort.Slice(bpList, func(i, j int) bool { return bpList[i].UpdatedAt > bpList[j].UpdatedAt })
	case "position":
		sort.Slice(bpList, func(i, j int) bool { return bpList[i].Position < bpList[j].Position })
	case "-position":
		sort.Slice(bpList, func(i, j int) bool { return bpList[i].Position > bpList[j].Position })
	default:
		return fmt.Errorf("unexpected order_by value %q", order)
	}
	return nil
}

func (h *Buildpack) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Buildpack) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: BuildpacksPath, Handler: h.list},
	}
}
