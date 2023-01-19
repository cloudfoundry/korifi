package controllers

import (
	"context"
	"fmt"

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
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, c.client, pdb, func() error {
		if pdb.Labels == nil {
			pdb.Labels = map[string]string{}
		}
		pdb.Labels[LabelGUID] = statefulSet.Labels[LabelGUID]
		pdb.Labels[LabelVersion] = statefulSet.Labels[LabelVersion]

		pdb.Spec = policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     statefulSet.Spec.Selector,
		}

		return controllerutil.SetOwnerReference(statefulSet, pdb, scheme.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to createOrPatch pod distruption budget: %w", err)
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
