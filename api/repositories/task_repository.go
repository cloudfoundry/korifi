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
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TaskResourceType string = "Task"
)

type TaskRecord struct {
	Name              string
	GUID              string
	Command           string
	AppGUID           string
	SequenceID        int64
	CreationTimestamp time.Time
	MemoryMB          int64
	DiskMB            int64
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
	userClientFactory authorization.UserK8sClientFactory
	timeout           time.Duration
}

func NewTaskRepo(userClientFactory authorization.UserK8sClientFactory, timeout time.Duration) *TaskRepo {
	return &TaskRepo{
		userClientFactory: userClientFactory,
		timeout:           timeout,
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

	return TaskRecord{
		Name:              task.Name,
		GUID:              task.Name,
		Command:           strings.Join(task.Spec.Command, " "),
		AppGUID:           task.Spec.AppRef.Name,
		SequenceID:        task.Status.SequenceID,
		CreationTimestamp: task.CreationTimestamp.Time,
		MemoryMB:          task.Status.MemoryMB,
		DiskMB:            task.Status.DiskQuotaMB,
	}, nil
}

func (r *TaskRepo) awaitInitialization(ctx context.Context, userClient client.WithWatch, task korifiv1alpha1.CFTask) (korifiv1alpha1.CFTask, error) {
	watch, err := userClient.Watch(ctx, &korifiv1alpha1.CFTaskList{}, client.InNamespace(task.Namespace), client.MatchingFields{"metadata.name": task.Name})
	if err != nil {
		return korifiv1alpha1.CFTask{}, apierrors.FromK8sError(err, TaskResourceType)
	}
	defer watch.Stop()

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
		case <-time.After(r.timeout):
			return korifiv1alpha1.CFTask{}, fmt.Errorf("task did not get initialized within timeout period %d ms", r.timeout.Milliseconds())
		}
	}
}

func splitCommand(command string) []string {
	whitespace := regexp.MustCompile(`\s+`)
	trimmedCommand := strings.TrimSpace(whitespace.ReplaceAllString(command, " "))
	return strings.Split(trimmedCommand, " ")
}
