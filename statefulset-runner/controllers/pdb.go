package controllers

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const PdbMinAvailableInstances = "50%"

type PDBUpdater struct {
	client client.Client
}

func NewPDBUpdater(client client.Client) *PDBUpdater {
	return &PDBUpdater{
		client: client,
	}
}

func (c *PDBUpdater) Update(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	if *statefulSet.Spec.Replicas > 1 {
		return c.createPDB(ctx, statefulSet)
	}

	return c.deletePDB(ctx, statefulSet)
}

func (c *PDBUpdater) createPDB(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	minAvailable := intstr.FromString(PdbMinAvailableInstances)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSet.Name,
			Namespace: statefulSet.Namespace,
			Labels: map[string]string{
				LabelGUID:    statefulSet.Labels[LabelGUID],
				LabelVersion: statefulSet.Labels[LabelVersion],
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     statefulSet.Spec.Selector,
		},
	}

	if err := controllerutil.SetOwnerReference(statefulSet, pdb, scheme.Scheme); err != nil {
		return fmt.Errorf("pdb updater failed to set owner ref : %w", err)
	}

	err := c.client.Create(ctx, pdb)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create pod distruption budget: %w", err)
	}

	return nil
}

func (c *PDBUpdater) deletePDB(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	err := c.client.DeleteAllOf(ctx, &policyv1.PodDisruptionBudget{}, client.InNamespace(statefulSet.Namespace), client.MatchingFields{"metadata.name": statefulSet.Name})
	if err != nil {
		return fmt.Errorf("failed to delete pod distruption budget: %w", err)
	}

	return nil
}
