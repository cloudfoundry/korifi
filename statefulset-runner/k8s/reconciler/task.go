package reconciler

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/lager"
	exterrors "github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Task struct {
	logger       lager.Logger
	client       client.Client
	desirer      TaskDesirer
	statusGetter TaskStatusGetter
	ttlSeconds   int
}

//counterfeiter:generate . TaskDesirer

type TaskDesirer interface {
	Desire(ctx context.Context, task *eiriniv1.Task) error
}

//counterfeiter:generate . TaskStatusGetter

type TaskStatusGetter interface {
	GetStatus(ctx context.Context, job *batchv1.Job) eiriniv1.TaskStatus
}

func NewTask(logger lager.Logger,
	client client.Client,
	desirer TaskDesirer,
	statusGetter TaskStatusGetter,
	ttlSeconds int) *Task {
	return &Task{
		logger:       logger,
		client:       client,
		desirer:      desirer,
		statusGetter: statusGetter,
		ttlSeconds:   ttlSeconds,
	}
}

func (t *Task) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := t.logger.Session("reconcile-task", lager.Data{"request": request})
	logger.Debug("start")

	task := &eiriniv1.Task{}

	err := t.client.Get(ctx, request.NamespacedName, task)
	if errors.IsNotFound(err) {
		logger.Debug("task-not-found")

		return reconcile.Result{}, nil
	}

	if err != nil {
		logger.Error("task-get-failed", err)

		return reconcile.Result{}, fmt.Errorf("could not fetch task: %w", err)
	}

	if taskHasCompleted(task) {
		return t.handleExpiredTask(ctx, logger, task)
	}

	job := &batchv1.Job{}

	err = t.client.Get(ctx, client.ObjectKey{Namespace: task.Namespace, Name: utils.GetJobName(task)}, job)
	if errors.IsNotFound(err) {
		desireErr := t.desirer.Desire(ctx, task)

		return reconcile.Result{}, exterrors.Wrap(desireErr, "failed to desire task")
	}

	if err != nil {
		return reconcile.Result{}, exterrors.Wrap(err, "failed to get job")
	}

	if err := t.updateTaskStatus(ctx, task, job); err != nil {
		return reconcile.Result{}, exterrors.Wrap(err, "failed to update task status")
	}

	if taskHasCompleted(task) {
		return reconcile.Result{RequeueAfter: time.Duration(t.ttlSeconds) * time.Second}, nil
	}

	return reconcile.Result{}, nil
}

func (t *Task) updateTaskStatus(ctx context.Context, task *eiriniv1.Task, job *batchv1.Job) error {
	originalTask := task.DeepCopy()

	status := t.statusGetter.GetStatus(ctx, job)
	task.Status = status

	return t.client.Status().Patch(ctx, task, client.MergeFrom(originalTask))
}

func (t *Task) handleExpiredTask(ctx context.Context, logger lager.Logger, task *eiriniv1.Task) (reconcile.Result, error) {
	if t.taskHasExpired(task) {
		logger.Debug("deleting-expired-job")

		err := t.client.DeleteAllOf(ctx, &batchv1.Job{}, client.InNamespace(task.Namespace), client.MatchingFields{"metadata.name": utils.GetJobName(task)})
		if err != nil {
			return reconcile.Result{}, exterrors.Wrap(err, "failed to delete job")
		}
	}

	logger.Debug("task-already-completed")

	return reconcile.Result{}, nil
}

func taskHasCompleted(task *eiriniv1.Task) bool {
	return task.Status.EndTime != nil &&
		(task.Status.ExecutionStatus == eiriniv1.TaskFailed ||
			task.Status.ExecutionStatus == eiriniv1.TaskSucceeded)
}

func (t *Task) taskHasExpired(task *eiriniv1.Task) bool {
	ttlExpire := metav1.NewTime(time.Now().Add(-time.Duration(t.ttlSeconds) * time.Second))

	return task.Status.EndTime.Before(&ttlExpire)
}
