package repositories

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"

	"github.com/BooleanCat/go-functional/v2/it/itx"
	corev1 "k8s.io/api/core/v1"
)

type PodRepo struct {
	klient Klient
}

func NewPodRepo(klient Klient) *PodRepo {
	return &PodRepo{
		klient: klient,
	}
}

func (r *PodRepo) DeletePod(ctx context.Context, authInfo authorization.Info, appRevision string, process ProcessRecord, instanceID string) error {
	podList := corev1.PodList{}
	err := r.klient.List(ctx, &podList,
		InNamespace(process.SpaceGUID),
		WithLabel("korifi.cloudfoundry.org/app-guid", process.AppGUID),
		WithLabel("korifi.cloudfoundry.org/version", appRevision),
		WithLabel("korifi.cloudfoundry.org/process-type", process.Type),
	)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	podsToDelete := itx.FromSlice(podList.Items).Filter(func(pod corev1.Pod) bool {
		return strings.HasSuffix(pod.Name, instanceID)
	}).Collect()

	if len(podsToDelete) == 0 {
		return apierrors.NewNotFoundError(nil, PodResourceType)
	}

	if len(podsToDelete) > 1 {
		return apierrors.NewUnprocessableEntityError(nil, "multiple pods found")
	}

	err = r.klient.Delete(ctx, &podsToDelete[0])
	if err != nil {
		return fmt.Errorf("failed to 'delete' pod: %w", apierrors.FromK8sError(err, PodResourceType))
	}
	return nil
}
