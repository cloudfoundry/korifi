package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"

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
}

type TaskHandler struct {
	serverURL        url.URL
	appRepo          CFAppRepository
	taskRepo         CFTaskRepository
	decoderValidator *DecoderValidator
}

func NewTaskHandler(
	serverURL url.URL,
	appRepo CFAppRepository,
	taskRepo CFTaskRepository,
	decoderValidator *DecoderValidator,
) *TaskHandler {
	return &TaskHandler{
		serverURL:        serverURL,
		taskRepo:         taskRepo,
		appRepo:          appRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *TaskHandler) taskGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	taskGUID := URLParam(r, "taskGUID")

	taskRecord, err := h.taskRepo.GetTask(ctx, authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get task", "taskGUID", taskGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) taskListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	tasks, err := h.taskRepo.ListTasks(ctx, authInfo, repositories.ListTaskMessage{})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list tasks")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTaskList(tasks, h.serverURL, *r.URL)), nil
}

func (h *TaskHandler) taskCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	appGUID := URLParam(r, "appGUID")

	var payload payloads.TaskCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	appRecord, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
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

	taskRecord, err := h.taskRepo.CreateTask(ctx, authInfo, payload.ToMessage(appRecord))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to create task")
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) appTaskListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	appGUID := URLParam(r, "appGUID")

	if err := r.ParseForm(); err != nil {
		logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	_, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding app", "appGUID", appGUID)
	}

	taskListFilter := new(payloads.TaskList)
	err = payloads.Decode(taskListFilter, r.Form)
	if err != nil {
		logger.Error(err, "Unable to decode request query parameters")
		return nil, err
	}

	taskListMessage := taskListFilter.ToMessage()
	taskListMessage.AppGUIDs = []string{appGUID}
	tasks, err := h.taskRepo.ListTasks(ctx, authInfo, taskListMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list tasks")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTaskList(tasks, h.serverURL, *r.URL)), nil
}

func (h *TaskHandler) cancelTaskHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	taskGUID := URLParam(r, "taskGUID")

	if _, err := h.taskRepo.GetTask(ctx, authInfo, taskGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding task", "taskGUID", taskGUID)
	}

	taskRecord, err := h.taskRepo.CancelTask(ctx, authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to cancel task")
	}

	return NewHandlerResponse(http.StatusAccepted).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}

func (h *TaskHandler) AuthenticatedRoutes() []AuthRoute {
	return []AuthRoute{
		{Method: "GET", Pattern: TaskPath, HandlerFunc: h.taskGetHandler},
		{Method: "GET", Pattern: TaskRoot, HandlerFunc: h.taskListHandler},
		{Method: "POST", Pattern: TasksPath, HandlerFunc: h.taskCreateHandler},
		{Method: "GET", Pattern: TasksPath, HandlerFunc: h.appTaskListHandler},
		{Method: "POST", Pattern: TaskCancelPath, HandlerFunc: h.cancelTaskHandler},
		{Method: "PUT", Pattern: TaskCancelPathDeprecated, HandlerFunc: h.cancelTaskHandler},
	}
}
