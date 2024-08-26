package repositories

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"

	"github.com/BooleanCat/go-functional/v2/it/itx"
	corev1 "k8s.io/api/core/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodRepo struct {
	userClientFactory authorization.UserK8sClientFactory
}

func NewPodRepo(userClientFactory authorization.UserK8sClientFactory) *PodRepo {
	return &PodRepo{
		userClientFactory: userClientFactory,
	}
}

func (r *PodRepo) DeletePod(ctx context.Context, authInfo authorization.Info, appRevision string, process ProcessRecord, instanceID string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		"korifi.cloudfoundry.org/app-guid":     process.AppGUID,
		"korifi.cloudfoundry.org/version":      appRevision,
		"korifi.cloudfoundry.org/process-type": process.Type,
	})
	if err != nil {
		return fmt.Errorf("failed to build labelSelector: %w", apierrors.FromK8sError(err, PodResourceType))
	}
	listOpts := client.ListOptions{Namespace: process.SpaceGUID, LabelSelector: labelSelector}

	podList := corev1.PodList{}
	err = userClient.List(ctx, &podList, &listOpts)
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
