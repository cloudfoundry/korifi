package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"

	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
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
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	serverURL        url.URL
	taskRepo         CFTaskRepository
	decoderValidator *DecoderValidator
}

func NewTaskHandler(
	serverURL url.URL,
	taskRepo CFTaskRepository,
	decoderValidator *DecoderValidator,
) *TaskHandler {
	return &TaskHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("TaskHandler")),
		serverURL:        serverURL,
		taskRepo:         taskRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *TaskHandler) taskGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	taskGUID := chi.URLParam(r, "taskGUID")

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

func (h *TaskHandler) cancelTaskHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	taskGUID := chi.URLParam(r, "taskGUID")

	if _, err := h.taskRepo.GetTask(ctx, authInfo, taskGUID); err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "error finding task", "taskGUID", taskGUID)
	}

	taskRecord, err := h.taskRepo.CancelTask(ctx, authInfo, taskGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to cancel task")
	}

	return NewHandlerResponse(http.StatusAccepted).WithBody(presenter.ForTask(taskRecord, h.serverURL)), nil
}

func (h *TaskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router := chi.NewRouter()
	router.Get(TaskPath, h.handlerWrapper.Wrap(h.taskGetHandler))
	router.Get(TaskRoot, h.handlerWrapper.Wrap(h.taskListHandler))
	router.Post(TaskCancelPath, h.handlerWrapper.Wrap(h.cancelTaskHandler))
	router.Put(TaskCancelPathDeprecated, h.handlerWrapper.Wrap(h.cancelTaskHandler))
	router.ServeHTTP(w, r)
}
