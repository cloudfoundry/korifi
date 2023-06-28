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

	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-logr/logr"
)

const (
	TasksPath                = "/v3/apps/{appGUID}/tasks"
	TaskRoot                 = "/v3/tasks"
	TaskPath                 = TaskRoot + "/{taskGUID}"
	TaskCancelPath           = TaskRoot + "/{taskGUID}/actions/cancel"
	TaskCancelPathDeprecated = TaskRoot + "/{taskGUID}/cancel"
)

//counterfeiter:generate -o fake -fake-name CFTaskRepository . CFTaskRepository
type CFTaskRepository interface {
	CreateTask(context.Context, authorization.Info, repositories.CreateTaskMessage) (repositories.TaskRecord, error)
	GetTask(context.Context, authorization.Info, string) (repositories.TaskRecord, error)
	ListTasks(context.Context, authorization.Info, repositories.ListTaskMessage) ([]repositories.TaskRecord, error)
	CancelTask(context.Context, authorization.Info, string) (repositories.TaskRecord, error)
	PatchTaskMetadata(ctx context.Context, info authorization.Info, message repositories.PatchTaskMetadataMessage) (repositories.TaskRecord, error)
}

type Task struct {
	serverURL        url.URL
	appRepo          CFAppRepository
	taskRepo         CFTaskRepository
	requestValidator RequestValidator
}

func NewTask(
	serverURL url.URL,
	appRepo CFAppRepository,
	taskRepo CFTaskRepository,
	requestValidator RequestValidator,
) *Task {
	return &Task{
		serverURL:        serverURL,
		taskRepo:         taskRepo,
		appRepo:          appRepo,
		requestValidator: requestValidator,
	}
}

func (h *Task) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.task.get")

	taskGUID := routing.URLParam(r, "taskGUID")

	taskRecord, err := h.taskRepo.GetTask(r.Context(), authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get task", "taskGUID", taskGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *Task) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.task.list")

	tasks, err := h.taskRepo.ListTasks(r.Context(), authInfo, repositories.ListTaskMessage{})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list tasks")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForTask, tasks, h.serverURL, *r.URL)), nil
}

func (h *Task) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.task.create")

	appGUID := routing.URLParam(r, "appGUID")

	var payload payloads.TaskCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	appRecord, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding app", "appGUID", appGUID)
	}

	if !appRecord.IsStaged {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(nil, "Task must have a droplet. Assign current droplet to app."),
			"app is not staged", "App GUID", appGUID,
		)
	}

	taskRecord, err := h.taskRepo.CreateTask(r.Context(), authInfo, payload.ToMessage(appRecord))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create task")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *Task) listForApp(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.task.list-for-app")

	appGUID := routing.URLParam(r, "appGUID")

	if _, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding app", "appGUID", appGUID)
	}

	taskListFilter := new(payloads.TaskList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, taskListFilter); err != nil {
		logger.Info("unable to decode request query parameters", "reason", err)
		return nil, err
	}

	taskListMessage := taskListFilter.ToMessage()
	taskListMessage.AppGUIDs = []string{appGUID}
	tasks, err := h.taskRepo.ListTasks(r.Context(), authInfo, taskListMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list tasks")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForTask, tasks, h.serverURL, *r.URL)), nil
}

func (h *Task) cancel(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.task.cancel")

	taskGUID := routing.URLParam(r, "taskGUID")

	if _, err := h.taskRepo.GetTask(r.Context(), authInfo, taskGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding task", "taskGUID", taskGUID)
	}

	taskRecord, err := h.taskRepo.CancelTask(r.Context(), authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to cancel task")
	}

	return routing.NewResponse(http.StatusAccepted).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

//nolint:dupl
func (h *Task) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.task.update")
	taskGUID := routing.URLParam(r, "taskGUID")

	task, err := h.taskRepo.GetTask(r.Context(), authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch task from Kubernetes", "TaskGUID", taskGUID)
	}

	var payload payloads.TaskUpdate
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	task, err = h.taskRepo.PatchTaskMetadata(r.Context(), authInfo, payload.ToMessage(taskGUID, task.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch task metadata", "TaskGUID", taskGUID)
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForTask(task, h.serverURL)), nil
}

func (h *Task) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Task) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: TaskPath, Handler: h.get},
		{Method: "PATCH", Pattern: TaskPath, Handler: h.update},
		{Method: "GET", Pattern: TaskRoot, Handler: h.list},
		{Method: "POST", Pattern: TasksPath, Handler: h.create},
		{Method: "GET", Pattern: TasksPath, Handler: h.listForApp},
		{Method: "POST", Pattern: TaskCancelPath, Handler: h.cancel},
		{Method: "PUT", Pattern: TaskCancelPathDeprecated, Handler: h.cancel},
	}
}
