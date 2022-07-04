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
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	TasksPath = "/v3/apps/{appGUID}/tasks"
	TaskRoot  = "/v3/tasks"
	TaskPath  = TaskRoot + "/{taskGUID}"
)

//counterfeiter:generate -o fake -fake-name CFTaskRepository . CFTaskRepository
type CFTaskRepository interface {
	CreateTask(context.Context, authorization.Info, repositories.CreateTaskMessage) (repositories.TaskRecord, error)
	GetTask(context.Context, authorization.Info, string) (repositories.TaskRecord, error)
	ListTasks(context.Context, authorization.Info, repositories.ListTaskMessage) ([]repositories.TaskRecord, error)
}

type TaskHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
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
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("TaskHandler")),
		serverURL:        serverURL,
		taskRepo:         taskRepo,
		appRepo:          appRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *TaskHandler) taskGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	taskGUID := vars["taskGUID"]

	taskRecord, err := h.taskRepo.GetTask(ctx, authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) taskListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	tasks, err := h.taskRepo.ListTasks(ctx, authInfo, repositories.ListTaskMessage{})
	if err != nil {
		logger.Error(err, "failed to list tasks")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTaskList(tasks, h.serverURL, *r.URL)), nil
}

func (h *TaskHandler) taskCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["appGUID"]

	var payload payloads.TaskCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	appRecord, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Info("Error finding App", "App GUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	if !appRecord.IsStaged {
		logger.Info("App is not staged", "App GUID", appGUID)
		return nil, apierrors.NewUnprocessableEntityError(nil, "Task must have a droplet. Assign current droplet to app.")
	}

	taskRecord, err := h.taskRepo.CreateTask(ctx, authInfo, payload.ToMessage(appRecord))
	if err != nil {
		logger.Error(err, "Failed to create task")
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) appTaskListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["appGUID"]

	tasks, err := h.taskRepo.ListTasks(ctx, authInfo, repositories.ListTaskMessage{
		AppGUIDs: []string{appGUID},
	})
	if err != nil {
		logger.Error(err, "failed to list tasks")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForTaskList(tasks, h.serverURL, *r.URL)), nil
}

func (h *TaskHandler) RegisterRoutes(router *mux.Router) {
	router.Path(TaskPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.taskGetHandler))
	router.Path(TaskRoot).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.taskListHandler))
	router.Path(TasksPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.taskCreateHandler))
	router.Path(TasksPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.appTaskListHandler))
}
