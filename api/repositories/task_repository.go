package repositories

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	TaskResourceType string = "Task"
)

type TaskRecord struct {
	Name    string
	GUID    string
	Command string
	AppGUID string
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
}

func NewTaskRepo(userClientFactory authorization.UserK8sClientFactory) *TaskRepo {
	return &TaskRepo{
		userClientFactory: userClientFactory,
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

	return TaskRecord{
		Name:    task.Name,
		GUID:    task.Name,
		Command: strings.Join(task.Spec.Command, " "),
		AppGUID: task.Spec.AppRef.Name,
	}, nil
}

func splitCommand(command string) []string {
	whitespace := regexp.MustCompile(`\s+`)
	trimmedCommand := strings.TrimSpace(whitespace.ReplaceAllString(command, " "))
	return strings.Split(trimmedCommand, " ")
}
