package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RunnerInfoResourceType = "RunnerInfo"
)

type RunnerInfoRepository struct {
	klient        Klient
	runnerName    string
	rootNamespace string
}

type RunnerInfoRecord struct {
	Name         string
	Namespace    string
	RunnerName   string
	Capabilities korifiv1alpha1.RunnerInfoCapabilities
}

func NewRunnerInfoRepository(klient Klient, runnerName string, rootNamespace string) *RunnerInfoRepository {
	return &RunnerInfoRepository{
		klient:        klient,
		runnerName:    runnerName,
		rootNamespace: rootNamespace,
	}
}

func (r *RunnerInfoRepository) GetRunnerInfo(ctx context.Context, authInfo authorization.Info, runnerName string) (RunnerInfoRecord, error) {
	runnerInfo := &korifiv1alpha1.RunnerInfo{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      r.runnerName,
		},
	}
	if err := r.klient.Get(ctx, runnerInfo); err != nil {
		return RunnerInfoRecord{}, fmt.Errorf("failed to get runner info: %w", apierrors.FromK8sError(err, RunnerInfoResourceType))
	}

	return r.runnerInfoToRunnerInfoRecord(*runnerInfo), nil
}

func (r *RunnerInfoRepository) runnerInfoToRunnerInfoRecord(info korifiv1alpha1.RunnerInfo) RunnerInfoRecord {
	return RunnerInfoRecord{
		Name:         info.Name,
		Namespace:    info.Namespace,
		RunnerName:   info.Spec.RunnerName,
		Capabilities: info.Status.Capabilities,
	}
}
