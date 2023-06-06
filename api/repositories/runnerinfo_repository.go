package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RunnerInfoResourceType = "RunnerInfo"
)

type RunnerInfoRepository struct {
	runnerName        string
	namespace         string
	userClientFactory authorization.UserK8sClientFactory
}

type RunnerInfoRecord struct {
	Name         string
	Namespace    string
	RunnerName   string
	Capabilities korifiv1alpha1.RunnerInfoCapabilities
}

func NewRunnerInfoRepository(userClientFactory authorization.UserK8sClientFactory, runnerName string, namespace string) *RunnerInfoRepository {
	return &RunnerInfoRepository{
		runnerName:        runnerName,
		namespace:         namespace,
		userClientFactory: userClientFactory,
	}
}

func (r *RunnerInfoRepository) GetRunnerInfo(ctx context.Context, authInfo authorization.Info, runnerName string) (RunnerInfoRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RunnerInfoRecord{}, err
	}

	runnerInfo := korifiv1alpha1.RunnerInfo{}
	if err = userClient.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: r.runnerName}, &runnerInfo); err != nil {
		return RunnerInfoRecord{}, fmt.Errorf("failed to get runner info: %w", apierrors.FromK8sError(err, RunnerInfoResourceType))
	}

	return r.runnerInfoToRunnerInfoRecord(runnerInfo), nil
}

func (r *RunnerInfoRepository) runnerInfoToRunnerInfoRecord(info korifiv1alpha1.RunnerInfo) RunnerInfoRecord {
	return RunnerInfoRecord{
		Name:         info.Name,
		Namespace:    info.Namespace,
		RunnerName:   info.Spec.RunnerName,
		Capabilities: info.Status.Capabilities,
	}
}
