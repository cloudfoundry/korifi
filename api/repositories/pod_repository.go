package repositories

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/BooleanCat/go-functional/v2/it/itx"
	corev1 "k8s.io/api/core/v1"
)

type PodRepo struct {
	userClientFactory authorization.UserClientFactory
}

func NewPodRepo(userClientFactory authorization.UserClientFactory) *PodRepo {
	return &PodRepo{
		userClientFactory: userClientFactory,
	}
}

func (r *PodRepo) DeletePod(ctx context.Context, authInfo authorization.Info, appRevision string, process ProcessRecord, instanceID string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList,
		client.InNamespace(process.SpaceGUID),
		client.MatchingLabels{
			"korifi.cloudfoundry.org/app-guid":     process.AppGUID,
			"korifi.cloudfoundry.org/version":      appRevision,
			"korifi.cloudfoundry.org/process-type": process.Type,
		},
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

	err = userClient.Delete(ctx, &podsToDelete[0])
	if err != nil {
		return fmt.Errorf("failed to 'delete' pod: %w", apierrors.FromK8sError(err, PodResourceType))
	}
	return nil
}
