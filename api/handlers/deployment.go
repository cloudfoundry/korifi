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
	DeploymentsPath = "/v3/deployments"
	DeploymentPath  = "/v3/deployments/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFDeploymentRepository . CFDeploymentRepository

type CFDeploymentRepository interface {
	GetDeployment(context.Context, authorization.Info, string) (repositories.DeploymentRecord, error)
	CreateDeployment(context.Context, authorization.Info, repositories.CreateDeploymentMessage) (repositories.DeploymentRecord, error)
}

type Deployment struct {
	serverURL            url.URL
	requestJSONValidator RequestJSONValidator
	deploymentRepo       CFDeploymentRepository
}

func NewDeployment(
	serverURL url.URL,
	requestJSONValidator RequestJSONValidator,
	deploymentRepo CFDeploymentRepository,
) *Deployment {
	return &Deployment{
		serverURL:            serverURL,
		requestJSONValidator: requestJSONValidator,
		deploymentRepo:       deploymentRepo,
	}
}

func (h *Deployment) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.deployment.create")

	var payload payloads.DeploymentCreate
	if err := h.requestJSONValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	deploymentCreateMessage := payload.ToMessage()

	deployment, err := h.deploymentRepo.CreateDeployment(r.Context(), authInfo, deploymentCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error creating deployment in repository")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForDeployment(deployment, h.serverURL)), nil
}

func (h *Deployment) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.deployment.get")

	deploymentGUID := routing.URLParam(r, "guid")

	deployment, err := h.deploymentRepo.GetDeployment(r.Context(), authInfo, deploymentGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting deployment in repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDeployment(deployment, h.serverURL)), nil
}

func (h *Deployment) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Deployment) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: DeploymentPath, Handler: h.get},
		{Method: "POST", Pattern: DeploymentsPath, Handler: h.create},
	}
}
