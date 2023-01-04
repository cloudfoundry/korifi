package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"

	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-chi/chi"
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

func (h *TaskHandler) taskGetHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("task-handler.task-get")

	taskGUID := chi.URLParam(r, "taskGUID")

	taskRecord, err := h.taskRepo.GetTask(r.Context(), authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get task", "taskGUID", taskGUID)
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) taskListHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("task-handler.task-list")

	tasks, err := h.taskRepo.ListTasks(r.Context(), authInfo, repositories.ListTaskMessage{})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list tasks")
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTaskList(tasks, h.serverURL, *r.URL)), nil
}

func (h *TaskHandler) taskCreateHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("task-handler.task-create")

	appGUID := chi.URLParam(r, "appGUID")

	var payload payloads.TaskCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	return routing.NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) appTaskListHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("task-handler.app-task-list")

	appGUID := chi.URLParam(r, "appGUID")

	if err := r.ParseForm(); err != nil {
		logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	_, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
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
	tasks, err := h.taskRepo.ListTasks(r.Context(), authInfo, taskListMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list tasks")
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTaskList(tasks, h.serverURL, *r.URL)), nil
}

func (h *TaskHandler) cancelTaskHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("task-handler.cancel-task")

	taskGUID := chi.URLParam(r, "taskGUID")

	if _, err := h.taskRepo.GetTask(r.Context(), authInfo, taskGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding task", "taskGUID", taskGUID)
	}

	taskRecord, err := h.taskRepo.CancelTask(r.Context(), authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to cancel task")
	}

	return routing.NewHandlerResponse(http.StatusAccepted).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", TaskPath, routing.Handler(h.taskGetHandler))
	router.Method("GET", TaskRoot, routing.Handler(h.taskListHandler))
	router.Method("POST", TasksPath, routing.Handler(h.taskCreateHandler))
	router.Method("GET", TasksPath, routing.Handler(h.appTaskListHandler))
	router.Method("POST", TaskCancelPath, routing.Handler(h.cancelTaskHandler))
	router.Method("PUT", TaskCancelPathDeprecated, routing.Handler(h.cancelTaskHandler))
}
