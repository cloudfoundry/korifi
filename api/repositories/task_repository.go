package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/resources"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/tasks"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
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
	TaskStateCanceling = "CANCELING"
)

type TaskRecord struct {
	Name          string
	GUID          string
	SpaceGUID     string
	Command       string
	AppGUID       string
	DropletGUID   string
	Labels        map[string]string
	Annotations   map[string]string
	SequenceID    int64
	CreatedAt     time.Time
	UpdatedAt     *time.Time
	MemoryMB      int64
	DiskMB        int64
	State         string
	FailureReason string
}

func (t TaskRecord) Relationships() map[string]string {
	return map[string]string{
		"app": t.AppGUID,
	}
}

type CreateTaskMessage struct {
	Command   string
	SpaceGUID string
	AppGUID   string
	Metadata
}

type ListTaskMessage struct {
	AppGUIDs    []string
	SequenceIDs []int64
}

func (m *ListTaskMessage) matches(task korifiv1alpha1.CFTask) bool {
	return tools.EmptyOrContains(m.SequenceIDs, task.Status.SequenceID) &&
		tools.EmptyOrContains(m.AppGUIDs, task.Spec.AppRef.Name)
}

type PatchTaskMetadataMessage struct {
	MetadataPatch
	TaskGUID  string
	SpaceGUID string
}

func (m *CreateTaskMessage) toCFTask() *korifiv1alpha1.CFTask {
	return &korifiv1alpha1.CFTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFTaskSpec{
			Command: m.Command,
			AppRef: v1.LocalObjectReference{
				Name: m.AppGUID,
			},
		},
	}
}

type TaskRepo struct {
	userClientFactory    authorization.UserClientFactory
	namespaceRetriever   NamespaceRetriever
	taskConditionAwaiter Awaiter[*korifiv1alpha1.CFTask]
}

func NewTaskRepo(
	userClientFactory authorization.UserClientFactory,
	nsRetriever NamespaceRetriever,
	taskConditionAwaiter Awaiter[*korifiv1alpha1.CFTask],
) *TaskRepo {
	return &TaskRepo{
		userClientFactory:    userClientFactory,
		namespaceRetriever:   nsRetriever,
		taskConditionAwaiter: taskConditionAwaiter,
	}
}

func (r *TaskRepo) CreateTask(ctx context.Context, authInfo authorization.Info, createMessage CreateTaskMessage) (TaskRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	task := createMessage.toCFTask()
	err = userClient.Create(ctx, task)
	if err != nil {
		return TaskRecord{}, apierrors.FromK8sError(err, TaskResourceType)
	}

	task, err = r.awaitCondition(ctx, userClient, task, korifiv1alpha1.TaskInitializedConditionType)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed waiting for task to get initialized: %w", err)
	}

	return taskToRecord(*task), nil
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

	cfTask := &korifiv1alpha1.CFTask{}
	err = userClient.Get(ctx, types.NamespacedName{Namespace: taskNamespace, Name: taskGUID}, cfTask)
	if err != nil {
		return TaskRecord{}, apierrors.FromK8sError(err, TaskResourceType)
	}

	// We cannot use IsStatusConditionFalse, because it would return false if
	// the condition were not present
	if !meta.IsStatusConditionTrue(cfTask.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType) {
		return TaskRecord{}, apierrors.NewNotFoundError(fmt.Errorf("task %s not initialized yet", taskGUID), TaskResourceType)
	}

	return taskToRecord(*cfTask), nil
}

func (r *TaskRepo) awaitCondition(ctx context.Context, userClient client.WithWatch, task *korifiv1alpha1.CFTask, conditionType string) (*korifiv1alpha1.CFTask, error) {
	awaitedTask, err := r.taskConditionAwaiter.AwaitCondition(ctx, resources.NewKlient(nil, r.userClientFactory), task, conditionType)
	if err != nil {
		return nil, apierrors.FromK8sError(err, TaskResourceType)
	}

	return awaitedTask, nil
}

func (r *TaskRepo) ListTasks(ctx context.Context, authInfo authorization.Info, msg ListTaskMessage) ([]TaskRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	taskList := &korifiv1alpha1.CFTaskList{}
	err = userClient.List(ctx, taskList)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", apierrors.FromK8sError(err, TaskResourceType))
	}

	filteredTasks := itx.FromSlice(taskList.Items).Filter(msg.matches)
	return slices.Collect(it.Map(filteredTasks, taskToRecord)), nil
}

func (r *TaskRepo) CancelTask(ctx context.Context, authInfo authorization.Info, taskGUID string) (TaskRecord, error) {
	taskNamespace, err := r.namespaceRetriever.NamespaceFor(ctx, taskGUID, TaskResourceType)
	if err != nil {
		return TaskRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	task := &korifiv1alpha1.CFTask{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: taskNamespace,
			Name:      taskGUID,
		},
	}
	err = k8s.PatchResource(ctx, userClient, task, func() {
		task.Spec.Canceled = true
	})
	if err != nil {
		return TaskRecord{}, apierrors.FromK8sError(err, TaskResourceType)
	}

	task, err = r.awaitCondition(ctx, userClient, task, korifiv1alpha1.TaskCanceledConditionType)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed waiting for task to get canceled: %w", err)
	}

	return taskToRecord(*task), nil
}

func (r *TaskRepo) PatchTaskMetadata(ctx context.Context, authInfo authorization.Info, message PatchTaskMetadataMessage) (TaskRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	task := new(korifiv1alpha1.CFTask)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: message.SpaceGUID, Name: message.TaskGUID}, task)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("failed to get task: %w", apierrors.FromK8sError(err, TaskResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, task, func() {
		message.Apply(task)
	})
	if err != nil {
		return TaskRecord{}, apierrors.FromK8sError(err, TaskResourceType)
	}

	return taskToRecord(*task), nil
}

func taskToRecord(task korifiv1alpha1.CFTask) TaskRecord {
	taskRecord := TaskRecord{
		Name:        task.Name,
		GUID:        task.Name,
		SpaceGUID:   task.Namespace,
		Command:     task.Spec.Command,
		AppGUID:     task.Spec.AppRef.Name,
		SequenceID:  task.Status.SequenceID,
		CreatedAt:   task.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(&task),
		MemoryMB:    task.Status.MemoryMB,
		DiskMB:      task.Status.DiskQuotaMB,
		DropletGUID: task.Status.DropletRef.Name,
		State:       toRecordState(&task),
		Labels:      task.Labels,
		Annotations: task.Annotations,
	}

	failedCond := meta.FindStatusCondition(task.Status.Conditions, korifiv1alpha1.TaskFailedConditionType)
	if failedCond != nil && failedCond.Status == metav1.ConditionTrue {
		taskRecord.FailureReason = failedCond.Message

		if failedCond.Reason == tasks.TaskCanceledReason {
			taskRecord.FailureReason = "task was cancelled"
		}
	}

	return taskRecord
}

func toRecordState(task *korifiv1alpha1.CFTask) string {
	switch {
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskSucceededConditionType):
		return TaskStateSucceeded
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskFailedConditionType):
		return TaskStateFailed
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskCanceledConditionType):
		return TaskStateCanceling
	case meta.IsStatusConditionTrue(task.Status.Conditions, korifiv1alpha1.TaskStartedConditionType):
		return TaskStateRunning
	default:
		return TaskStatePending
	}
}
