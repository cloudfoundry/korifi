/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8sns

import (
	"context"
	"fmt"
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1/status"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type NamespaceObject[T any] interface {
	*T
	client.Object
	GetStatus() status.NamespaceStatus
}

type MetadataCompiler[T any, NS NamespaceObject[T]] interface {
	CompileLabels(NS) map[string]string
	CompileAnnotations(NS) map[string]string
}

type Reconciler[T any, NS NamespaceObject[T]] struct {
	client                       client.Client
	finalizer                    Finalizer[T, NS]
	containerRegistrySecretNames []string
	metadataCompiler             MetadataCompiler[T, NS]
}

func NewReconciler[T any, NS NamespaceObject[T]](
	client client.Client,
	finalizer Finalizer[T, NS],
	metadataCompiler MetadataCompiler[T, NS],
	containerRegistrySecretNames []string,
) *Reconciler[T, NS] {
	return &Reconciler[T, NS]{
		client:                       client,
		finalizer:                    finalizer,
		metadataCompiler:             metadataCompiler,
		containerRegistrySecretNames: containerRegistrySecretNames,
	}
}

func (r *Reconciler[T, NS]) ReconcileResource(ctx context.Context, obj NS) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	obj.GetStatus().SetObservedGeneration(obj.GetGeneration())
	log.V(1).Info("set observed generation", "generation", obj.GetGeneration())

	if !obj.GetDeletionTimestamp().IsZero() {
		return r.finalizer.Finalize(ctx, obj)
	}

	shared.GetConditionOrSetAsUnknown(obj.GetStatus().GetConditions(), korifiv1alpha1.ReadyConditionType, obj.GetGeneration())

	obj.GetStatus().SetGUID(obj.GetName())

	err := r.createOrPatchNamespace(ctx, obj)
	if err != nil {
		return r.setNotReady(log, obj, fmt.Errorf("error creating namespace: %w", err), "NamespaceCreation")
	}

	err = r.getNamespace(ctx, obj.GetName())
	if err != nil {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = r.propagateSecrets(ctx, obj, r.containerRegistrySecretNames)
	if err != nil {
		return r.setNotReady(log, obj, fmt.Errorf("error propagating secrets: %w", err), "RegistrySecretPropagation")
	}

	err = r.reconcileRoleBindings(ctx, obj)
	if err != nil {
		return r.setNotReady(log, obj, fmt.Errorf("error propagating role-bindings: %w", err), "RoleBindingPropagation")
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler[T, NS]) createOrPatchNamespace(ctx context.Context, obj NS) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchNamespace")

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: obj.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, namespace, func() error {
		updateMap(&namespace.Annotations, r.metadataCompiler.CompileAnnotations(obj))
		updateMap(&namespace.Labels, r.metadataCompiler.CompileLabels(obj))
		return nil
	})
	if err != nil {
		return err
	}

	log.V(1).Info("Namespace reconciled", "operation", result)
	return nil
}

func updateMap(dest *map[string]string, values map[string]string) {
	if *dest == nil {
		*dest = make(map[string]string)
	}

	for key, value := range values {
		(*dest)[key] = value
	}
}

func (r *Reconciler[T, NS]) setNotReady(log logr.Logger, obj NS, err error, reason string) (ctrl.Result, error) {
	log.Info("not ready yet", "reason", reason, "error", err)

	meta.SetStatusCondition(obj.GetStatus().GetConditions(), metav1.Condition{
		Type:               shared.StatusConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		ObservedGeneration: obj.GetGeneration(),
	})

	return ctrl.Result{}, err
}

func (r *Reconciler[T, NS]) propagateSecrets(ctx context.Context, obj NS, secretNames []string) error {
	if len(secretNames) == 0 {
		// we are operating in service account role association mode for registry permissions.
		// only tested implicity in EKS e2es
		return nil
	}

	log := logr.FromContextOrDiscard(ctx).WithName("propagateSecret").
		WithValues("parentNamespace", obj.GetNamespace(), "targetNamespace", obj.GetName())

	for _, secretName := range secretNames {
		looplog := log.WithValues("secretName", secretName)

		secret := new(corev1.Secret)
		err := r.client.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: secretName}, secret)
		if err != nil {
			return fmt.Errorf("error fetching secret %q from namespace %q: %w", secretName, obj.GetNamespace(), err)
		}

		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: obj.GetName(),
			},
		}

		result, err := controllerutil.CreateOrPatch(ctx, r.client, newSecret, func() error {
			newSecret.Annotations = removePackageManagerKeys(secret.Annotations, looplog)
			newSecret.Labels = removePackageManagerKeys(secret.Labels, looplog)
			newSecret.Immutable = secret.Immutable
			newSecret.Data = secret.Data
			newSecret.StringData = secret.StringData
			newSecret.Type = secret.Type
			return nil
		})
		if err != nil {
			return fmt.Errorf("error creating/patching secret: %w", err)
		}

		looplog.V(1).Info("Secret propagated", "operation", result)
	}

	log.V(1).Info("All secrets propagated")

	return nil
}

func removePackageManagerKeys(src map[string]string, log logr.Logger) map[string]string {
	if src == nil {
		return src
	}

	dest := map[string]string{}
	for key, value := range src {
		if strings.HasPrefix(key, "meta.helm.sh/") {
			log.V(1).Info("skipping helm annotation propagation", "key", key)
			continue
		}

		if strings.HasPrefix(key, "kapp.k14s.io/") {
			log.V(1).Info("skipping kapp annotation propagation", "key", key)
			continue
		}

		dest[key] = value
	}

	return dest
}

func (r *Reconciler[T, NS]) reconcileRoleBindings(ctx context.Context, obj NS) error {
	log := logr.FromContextOrDiscard(ctx).WithName("propagateRolebindings").
		WithValues("parentNamespace", obj.GetNamespace(), "targetNamespace", obj.GetName())

	roleBindings := new(rbacv1.RoleBindingList)
	err := r.client.List(ctx, roleBindings, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		log.Info("error listing role-bindings from the parent namespace", "reason", err)
		return err
	}

	var result controllerutil.OperationResult
	parentRoleBindingMap := make(map[string]struct{})
	for _, binding := range roleBindings.Items {
		if binding.Annotations[korifiv1alpha1.PropagateRoleBindingAnnotation] == "true" {
			loopLog := log.WithValues("roleBindingName", binding.Name)

			parentRoleBindingMap[binding.Name] = struct{}{}

			newRoleBinding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Name,
					Namespace: obj.GetName(),
				},
			}

			result, err = controllerutil.CreateOrPatch(ctx, r.client, newRoleBinding, func() error {
				newRoleBinding.Annotations = shared.RemovePackageManagerKeys(binding.Annotations, log)
				newRoleBinding.Labels = shared.RemovePackageManagerKeys(binding.Labels, log)
				if newRoleBinding.Labels == nil {
					newRoleBinding.Labels = map[string]string{}
				}
				newRoleBinding.Labels[korifiv1alpha1.PropagatedFromLabel] = obj.GetNamespace()
				newRoleBinding.Subjects = binding.Subjects
				newRoleBinding.RoleRef = binding.RoleRef
				return nil
			})
			if err != nil {
				loopLog.Info("error propagating role-binding", "reason", err)
				return err
			}

			loopLog.V(1).Info("Role Binding propagated", "operation", result)
		}
	}

	propagatedRoleBindings := new(rbacv1.RoleBindingList)
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.PropagatedFromLabel: obj.GetNamespace(),
	})
	if err != nil {
		log.Info("failed to create label selector", "reason", err)
		return err
	}

	err = r.client.List(ctx, propagatedRoleBindings, &client.ListOptions{Namespace: obj.GetName(), LabelSelector: labelSelector})
	if err != nil {
		log.Info("error listing role-bindings from parent namespace", "reason", err)
		return err
	}

	for index := range propagatedRoleBindings.Items {
		propagatedRoleBinding := propagatedRoleBindings.Items[index]
		if propagatedRoleBinding.Annotations[korifiv1alpha1.PropagateDeletionAnnotation] == "false" {
			continue
		}

		if _, found := parentRoleBindingMap[propagatedRoleBinding.Name]; !found {
			err = r.client.Delete(ctx, &propagatedRoleBinding)
			if err != nil {
				log.Info("deleting role binding from target namespace failed", "roleBindingName", propagatedRoleBinding.Name, "reason", err)
				return err
			}
		}
	}

	return nil
}

func (r *Reconciler[T, NS]) getNamespace(ctx context.Context, namespaceName string) error {
	log := logr.FromContextOrDiscard(ctx).WithValues("namespace", namespaceName)

	namespace := new(corev1.Namespace)
	err := r.client.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
	if err != nil {
		log.Info("failed to get namespace", "reason", err)
		return err
	}

	return nil
}
