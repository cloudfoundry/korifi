package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	Name              string
	GUID              string
	SpaceGUID         string
	Command           string
	AppGUID           string
	DropletGUID       string
	Labels            map[string]string
	Annotations       map[string]string
	SequenceID        int64
	CreationTimestamp time.Time
	MemoryMB          int64
	DiskMB            int64
	State             string
	FailureReason     string
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

type PatchTaskMetadataMessage struct {
	MetadataPatch
	TaskGUID  string
	SpaceGUID string
}

func (m *CreateTaskMessage) toCFTask() *korifiv1alpha1.CFTask {
	guid := uuid.NewString()

	return &korifiv1alpha1.CFTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
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
	userClientFactory    authorization.UserK8sClientFactory
	namespaceRetriever   NamespaceRetriever
	namespacePermissions *authorization.NamespacePermissions
	taskConditionAwaiter ConditionAwaiter[*korifiv1alpha1.CFTask]
}

func NewTaskRepo(
	userClientFactory authorization.UserK8sClientFactory,
	nsRetriever NamespaceRetriever,
	namespacePermissions *authorization.NamespacePermissions,
	taskConditionAwaiter ConditionAwaiter[*korifiv1alpha1.CFTask],
) *TaskRepo {
	return &TaskRepo{
		userClientFactory:    userClientFactory,
		namespaceRetriever:   nsRetriever,
		namespacePermissions: namespacePermissions,
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

	return taskToRecord(cfTask), nil
}

func (r *TaskRepo) awaitCondition(ctx context.Context, userClient client.WithWatch, task *korifiv1alpha1.CFTask, conditionType string) (*korifiv1alpha1.CFTask, error) {
	awaitedTask, err := r.taskConditionAwaiter.AwaitCondition(ctx, userClient, task, conditionType)
	if err != nil {
		return nil, apierrors.FromK8sError(err, TaskResourceType)
	}

	return awaitedTask, nil
}

func (r *TaskRepo) ListTasks(ctx context.Context, authInfo authorization.Info, msg ListTaskMessage) ([]TaskRecord, error) {
	nsList, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	var tasks []korifiv1alpha1.CFTask
	for ns := range nsList {
		taskList := &korifiv1alpha1.CFTaskList{}
		err := userClient.List(ctx, taskList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list tasks in namespace %s: %w", ns, apierrors.FromK8sError(err, TaskResourceType))
		}
		tasks = append(tasks, filterBySequenceIDs(filterByAppGUIDs(taskList.Items, msg.AppGUIDs), msg.SequenceIDs)...)
	}

	taskRecords := []TaskRecord{}
	for i := range tasks {
		taskRecords = append(taskRecords, taskToRecord(&tasks[i]))
	}

	return taskRecords, nil
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

	return taskToRecord(task), nil
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

	return taskToRecord(task), nil
}

func filterByAppGUIDs(tasks []korifiv1alpha1.CFTask, appGUIDs []string) []korifiv1alpha1.CFTask {
	if len(appGUIDs) == 0 {
		return tasks
	}

	guidMap := map[string]bool{}
	for _, g := range appGUIDs {
		guidMap[g] = true
	}

	var res []korifiv1alpha1.CFTask
	for _, t := range tasks {
		if guidMap[t.Spec.AppRef.Name] {
			res = append(res, t)
		}
	}

	return res
}

func filterBySequenceIDs(tasks []korifiv1alpha1.CFTask, sequenceIDs []int64) []korifiv1alpha1.CFTask {
	if len(sequenceIDs) == 0 {
		return tasks
	}

	seqIdMap := map[int64]bool{}
	for _, seqId := range sequenceIDs {
		seqIdMap[seqId] = true
	}

	var res []korifiv1alpha1.CFTask
	for _, t := range tasks {
		if seqIdMap[t.Status.SequenceID] {
			res = append(res, t)
		}
	}

	return res
}

func taskToRecord(task *korifiv1alpha1.CFTask) TaskRecord {
	taskRecord := TaskRecord{
		Name:              task.Name,
		GUID:              task.Name,
		SpaceGUID:         task.Namespace,
		Command:           task.Spec.Command,
		AppGUID:           task.Spec.AppRef.Name,
		SequenceID:        task.Status.SequenceID,
		CreationTimestamp: task.CreationTimestamp.Time,
		MemoryMB:          task.Status.MemoryMB,
		DiskMB:            task.Status.DiskQuotaMB,
		DropletGUID:       task.Status.DropletRef.Name,
		State:             toRecordState(task),
		Labels:            task.Labels,
		Annotations:       task.Annotations,
	}

	failedCond := meta.FindStatusCondition(task.Status.Conditions, korifiv1alpha1.TaskFailedConditionType)
	if failedCond != nil && failedCond.Status == metav1.ConditionTrue {
		taskRecord.FailureReason = failedCond.Message

		if failedCond.Reason == workloads.TaskCanceledReason {
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
