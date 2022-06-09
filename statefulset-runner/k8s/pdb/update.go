package pdb

import (
	"context"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/policy/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const PdbMinAvailableInstances = "50%"

type Updater struct {
	client client.Client
}

func NewUpdater(client client.Client) *Updater {
	return &Updater{
		client: client,
	}
}

func (c *Updater) Update(ctx context.Context, statefulSet *appsv1.StatefulSet, lrp *eiriniv1.LRP) error {
	if lrp.Spec.Instances > 1 {
		return c.createPDB(ctx, statefulSet, lrp)
	}

	return c.deletePDB(ctx, statefulSet)
}

func (c *Updater) createPDB(ctx context.Context, statefulSet *appsv1.StatefulSet, lrp *eiriniv1.LRP) error {
	minAvailable := intstr.FromString(PdbMinAvailableInstances)

	pdb := &v1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSet.Name,
			Namespace: statefulSet.Namespace,
			Labels: map[string]string{
				stset.LabelGUID:    lrp.Spec.GUID,
				stset.LabelVersion: lrp.Spec.Version,
			},
		},
		Spec: v1beta1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     stset.StatefulSetLabelSelector(lrp),
		},
	}

	if err := controllerutil.SetOwnerReference(statefulSet, pdb, scheme.Scheme); err != nil {
		return errors.Wrap(err, "pdb-updated-failed-to-set-owner-ref")
	}

	err := c.client.Create(ctx, pdb)

	if k8serrors.IsAlreadyExists(err) {
		return nil
	}

	return errors.Wrap(err, "failed to create pod distruption budget")
}

func (c *Updater) deletePDB(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	err := c.client.DeleteAllOf(ctx, &v1beta1.PodDisruptionBudget{}, client.InNamespace(statefulSet.Namespace), client.MatchingFields{"metadata.name": statefulSet.Name})

	return errors.Wrap(err, "failed to delete pod distruption budget")
}
