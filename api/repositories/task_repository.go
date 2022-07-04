package repositories

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TaskResourceType string = "Task"

	TaskStatePending   = "PENDING"
	TaskStateRunning   = "RUNNING"
	TaskStateSucceeded = "SUCCEEDED"
	TaskStateFailed    = "FAILED"
)

type TaskRecord struct {
	Name              string
	GUID              string
	Command           string
	AppGUID           string
	DropletGUID       string
	SequenceID        int64
	CreationTimestamp time.Time
	MemoryMB          int64
	DiskMB            int64
	State             string
}

type CreateTaskMessage struct {
	Command   string
	SpaceGUID string
	AppGUID   string
}

func (m *CreateTaskMessage) toCFTask() korifiv1alpha1.CFTask {
	guid := uuid.NewString()

	return korifiv1alpha1.CFTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: m.SpaceGUID,
		},
		Spec: korifiv1alpha1.CFTaskSpec{
			Command: splitCommand(m.Command),
			AppRef: v1.LocalObjectReference{
				Name: m.AppGUID,
			},
		},
	}
}

type TaskRepo struct {
	userClientFactory  authorization.UserK8sClientFactory
	namespaceRetriever NamespaceRetriever
	timeout            time.Duration
}

func NewTaskRepo(userClientFactory authorization.UserK8sClientFactory, nsRetriever NamespaceRetriever, timeout time.Duration) *TaskRepo {
	return &TaskRepo{
		userClientFactory:  userClientFactory,
		namespaceRetriever: nsRetriever,
		timeout:            timeout,
	}
}

func (r *TaskRepo) CreateTask(ctx context.Context, authInfo authorization.Info, createMessage CreateTaskMessage) (TaskRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	task := createMessage.toCFTask()
	err = userClient.Create(ctx, &task)
	if err != nil {
		return TaskRecord{}, apierrors.FromK8sError(err, TaskResourceType)
	}

	task, err = r.awaitInitialization(ctx, userClient, task)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed waiting for task to get initialized: %w", err)
	}

	return taskToRecord(task), nil
}

func (r *TaskRepo) GetTask(ctx context.Context, authInfo authorization.Info, taskGUID string) (TaskRecord, error) {
	taskNamespace, err := r.namespaceRetriever.NamespaceFor(ctx, taskGUID, TaskResourceType)
	if err != nil {
		return TaskRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfTask := korifiv1alpha1.CFTask{}
	err = userClient.Get(ctx, types.NamespacedName{Namespace: taskNamespace, Name: taskGUID}, &cfTask)
	if err != nil {
		return TaskRecord{}, apierrors.FromK8sError(err, TaskResourceType)
	}

	// We cannot use IsStatusConditionFalse, because it would return false if
	// the condition were not present
	if !meta.IsStatusConditionTrue(cfTask.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType) {
		return TaskRecord{}, apierrors.NewNotFoundError(fmt.Errorf("task %s not initialized yet", taskGUID), TaskResourceType)
	}

	return taskToRecord(cfTask), nil
}

func (r *TaskRepo) awaitInitialization(ctx context.Context, userClient client.WithWatch, task korifiv1alpha1.CFTask) (korifiv1alpha1.CFTask, error) {
	watch, err := userClient.Watch(ctx, &korifiv1alpha1.CFTaskList{}, client.InNamespace(task.Namespace), client.MatchingFields{"metadata.name": task.Name})
	if err != nil {
		return korifiv1alpha1.CFTask{}, apierrors.FromK8sError(err, TaskResourceType)
	}
	defer watch.Stop()

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()

	for {
		select {
		case e := <-watch.ResultChan():
			updatedTask, ok := e.Object.(*korifiv1alpha1.CFTask)
			if !ok {
				continue
			}

			if meta.IsStatusConditionTrue(updatedTask.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType) {
				return *updatedTask, nil
			}
		case <-timer.C:
			return korifiv1alpha1.CFTask{}, fmt.Errorf("task did not get initialized within timeout period %d ms", r.timeout.Milliseconds())
		}
	}
}

func splitCommand(command string) []string {
	whitespace := regexp.MustCompile(`\s+`)
	trimmedCommand := strings.TrimSpace(whitespace.ReplaceAllString(command, " "))
	return strings.Split(trimmedCommand, " ")
}

func taskToRecord(task korifiv1alpha1.CFTask) TaskRecord {
	return TaskRecord{
		Name:              task.Name,
		GUID:              task.Name,
		Command:           strings.Join(task.Spec.Command, " "),
		AppGUID:           task.Spec.AppRef.Name,
		SequenceID:        task.Status.SequenceID,
		CreationTimestamp: task.CreationTimestamp.Time,
		MemoryMB:          task.Status.MemoryMB,
		DiskMB:            task.Status.DiskQuotaMB,
		DropletGUID:       task.Status.DropletRef.Name,
		State:             toRecordState(task),
	}
}

func toRecordState(task korifiv1alpha1.CFTask) string {
	switch {
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskSucceededConditionType):
		return TaskStateSucceeded
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskFailedConditionType):
		return TaskStateFailed
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskStartedConditionType):
		return TaskStateRunning
	default:
		return TaskStatePending
	}
}
