package handlers

import (
	"context"
	"errors"
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
	ListDeployments(context.Context, authorization.Info, repositories.ListDeploymentsMessage) (repositories.ListResult[repositories.DeploymentRecord], error)
}

//counterfeiter:generate -o fake -fake-name RunnerInfoRepository . RunnerInfoRepository

type RunnerInfoRepository interface {
	GetRunnerInfo(context.Context, authorization.Info, string) (repositories.RunnerInfoRecord, error)
}

type Deployment struct {
	serverURL        url.URL
	requestValidator RequestValidator
	deploymentRepo   CFDeploymentRepository
	runnerInfoRepo   RunnerInfoRepository
	runnerName       string
}

func NewDeployment(
	serverURL url.URL,
	requestValidator RequestValidator,
	deploymentRepo CFDeploymentRepository,
	runnerInfoRepo RunnerInfoRepository,
	runnerName string,
) *Deployment {
	return &Deployment{
		serverURL:        serverURL,
		requestValidator: requestValidator,
		deploymentRepo:   deploymentRepo,
		runnerInfoRepo:   runnerInfoRepo,
		runnerName:       runnerName,
	}
}

func (h *Deployment) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.deployment.create")

	var payload payloads.DeploymentCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	runnerInfo, err := h.runnerInfoRepo.GetRunnerInfo(r.Context(), authInfo, h.runnerName)
	if err != nil {
		var notFoundErr apierrors.NotFoundError
		if errors.As(err, &notFoundErr) {
			logger.Info("Could not find RunnerInfo", "runner", h.runnerName, "error", err)
			return nil, apierrors.NewRollingDeployNotSupportedError(h.runnerName)
		}
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting runner info in repository")
	}

	if !runnerInfo.Capabilities.RollingDeploy {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewRollingDeployNotSupportedError(h.runnerName), "runner does not support rolling deploys", "name", h.runnerName)
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

func (h *Deployment) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.deployment.list")

	payload := new(payloads.DeploymentList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	deployments, err := h.deploymentRepo.ListDeployments(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch deployments from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForDeployment, deployments, h.serverURL, *r.URL)), nil
}

func (h *Deployment) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Deployment) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: DeploymentPath, Handler: h.get},
		{Method: "POST", Pattern: DeploymentsPath, Handler: h.create},
		{Method: "GET", Pattern: DeploymentsPath, Handler: h.list},
	}
}
