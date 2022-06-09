package stset

import (
	"context"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Updater struct {
	logger     lager.Logger
	client     client.Client
	pdbUpdater PodDisruptionBudgetUpdater
}

func NewUpdater(logger lager.Logger, client client.Client, pdbUpdater PodDisruptionBudgetUpdater) *Updater {
	return &Updater{
		logger:     logger,
		client:     client,
		pdbUpdater: pdbUpdater,
	}
}

func (u *Updater) Update(ctx context.Context, lrp *eiriniv1.LRP, stSet *appsv1.StatefulSet) error {
	logger := u.logger.Session("update", lager.Data{"guid": lrp.Spec.GUID, "version": lrp.Spec.Version})

	updatedStatefulSet, updated := u.getUpdatedStatefulSetObj(stSet, lrp.Spec.Instances, lrp.Spec.Image)

	if !updated {
		return nil
	}

	if err := u.client.Patch(ctx, updatedStatefulSet, client.MergeFrom(stSet)); err != nil {
		logger.Error("failed-to-patch-statefulset", err, lager.Data{"namespace": stSet.Namespace})

		return errors.Wrap(err, "failed to patch statefulset")
	}

	if err := u.pdbUpdater.Update(ctx, stSet, lrp); err != nil {
		logger.Error("failed-to-update-disruption-budget", err, lager.Data{"namespace": stSet.Namespace})

		return errors.Wrap(err, "failed to delete pod disruption budget")
	}

	return nil
}

func (u *Updater) getUpdatedStatefulSetObj(sts *appsv1.StatefulSet, instances int, image string) (*appsv1.StatefulSet, bool) {
	updated := false

	updatedSts := sts.DeepCopy()

	if count := int32(instances); *sts.Spec.Replicas != count {
		updated = true
		updatedSts.Spec.Replicas = &count
	}

	if image != "" {
		for i, container := range updatedSts.Spec.Template.Spec.Containers {
			if container.Name == ApplicationContainerName && sts.Spec.Template.Spec.Containers[i].Image != image {
				updated = true
				updatedSts.Spec.Template.Spec.Containers[i].Image = image
			}
		}
	}

	return updatedSts, updated
}
