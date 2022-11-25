package workloads

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/pod-security-admission/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

func createOrPatchNamespace(ctx context.Context, client client.Client, log logr.Logger, orgOrSpace client.Object, labels map[string]string) error {
	log = log.WithName("createOrPatchNamespace")

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: orgOrSpace.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, client, namespace, func() error {
		if namespace.Labels == nil {
			namespace.Labels = make(map[string]string)
		}

		for key, value := range labels {
			namespace.Labels[key] = value
		}
		namespace.Labels[api.EnforceLevelLabel] = string(api.LevelRestricted)
		namespace.Labels[api.AuditLevelLabel] = string(api.LevelRestricted)

		return nil
	})
	if err != nil {
		return err
	}

	log.Info("Namespace reconciled", "operation", result)
	return nil
}

func propagateSecret(ctx context.Context, client client.Client, log logr.Logger, orgOrSpace client.Object, secretName string) error {
	if secretName == "" {
		// we are operating in service account role association mode for registry permissions.
		// only tested implicity in EKS e2es
		return nil
	}

	log = log.WithName("propagateSecret").
		WithValues("secretName", secretName, "parentNamespace", orgOrSpace.GetNamespace(), "targetNamespace", orgOrSpace.GetName())

	secret := new(corev1.Secret)
	err := client.Get(ctx, types.NamespacedName{Namespace: orgOrSpace.GetNamespace(), Name: secretName}, secret)
	if err != nil {
		log.Error(err, "Error fetching secret from parent namespace")
		return err
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: orgOrSpace.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, client, newSecret, func() error {
		newSecret.Annotations = secret.Annotations
		newSecret.Labels = secret.Labels
		newSecret.Immutable = secret.Immutable
		newSecret.Data = secret.Data
		newSecret.StringData = secret.StringData
		newSecret.Type = secret.Type
		return nil
	})
	if err != nil {
		log.Error(err, "Error creating/patching secret")
		return err
	}

	log.Info("Secret propagated", "operation", result)

	return nil
}

func reconcileRoleBindings(ctx context.Context, kClient client.Client, log logr.Logger, orgOrSpace client.Object) error {
	var (
		result controllerutil.OperationResult
		err    error
	)

	log = log.WithName("propagateRolebindings").
		WithValues("parentNamespace", orgOrSpace.GetNamespace(), "targetNamespace", orgOrSpace.GetName())

	roleBindings := new(rbacv1.RoleBindingList)
	err = kClient.List(ctx, roleBindings, client.InNamespace(orgOrSpace.GetNamespace()))
	if err != nil {
		log.Error(err, "Error listing role-bindings from the parent namespace")
		return err
	}

	parentRoleBindingMap := make(map[string]struct{})
	for _, binding := range roleBindings.Items {
		if binding.Annotations[korifiv1alpha1.PropagateRoleBindingAnnotation] == "true" {
			loopLog := log.WithValues("roleBindingName", binding.Name)

			parentRoleBindingMap[binding.Name] = struct{}{}

			newRoleBinding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Name,
					Namespace: orgOrSpace.GetName(),
				},
			}

			result, err = controllerutil.CreateOrPatch(ctx, kClient, newRoleBinding, func() error {
				newRoleBinding.Labels = binding.Labels
				if newRoleBinding.Labels == nil {
					newRoleBinding.Labels = map[string]string{}
				}
				newRoleBinding.Labels[korifiv1alpha1.PropagatedFromLabel] = orgOrSpace.GetNamespace()
				newRoleBinding.Annotations = binding.Annotations
				newRoleBinding.Subjects = binding.Subjects
				newRoleBinding.RoleRef = binding.RoleRef
				return nil
			})
			if err != nil {
				loopLog.Error(err, "Error propagting role-binding")
				return err
			}

			loopLog.Info("Role Binding propagated", "operation", result)
		}
	}

	propagatedRoleBindings := new(rbacv1.RoleBindingList)
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.PropagatedFromLabel: orgOrSpace.GetNamespace(),
	})
	if err != nil {
		return err
	}

	err = kClient.List(ctx, propagatedRoleBindings, &client.ListOptions{Namespace: orgOrSpace.GetName(), LabelSelector: labelSelector})
	if err != nil {
		log.Error(err, "Error listing role-bindings from parent namespace")
		return err
	}

	for index := range propagatedRoleBindings.Items {
		propagatedRoleBinding := propagatedRoleBindings.Items[index]
		if _, found := parentRoleBindingMap[propagatedRoleBinding.Name]; !found {
			err = kClient.Delete(ctx, &propagatedRoleBinding)
			if err != nil {
				log.Error(err, "deleting role binding from target namespace failed", "roleBindingName", propagatedRoleBinding.Name)
				return err
			}
		}
	}
	return nil
}

func finalize(ctx context.Context, kClient client.Client, log logr.Logger, orgOrSpace client.Object, finalizerName string) (ctrl.Result, error) {
	log = log.WithName("finalize")

	if !controllerutil.ContainsFinalizer(orgOrSpace, finalizerName) {
		return ctrl.Result{}, nil
	}

	err := kClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: orgOrSpace.GetName()}})
	if err != nil {
		log.Error(err, "Failed to delete namespace")
		return ctrl.Result{}, err
	}

	if controllerutil.RemoveFinalizer(orgOrSpace, finalizerName) {
		log.Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}

func getNamespace(ctx context.Context, log logr.Logger, client client.Client, namespaceName string) (*corev1.Namespace, bool) {
	log = log.WithValues("namespace", namespaceName)

	namespace := new(corev1.Namespace)
	err := client.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
	if err != nil {
		log.Error(err, "failed to get namespace")
		return nil, false
	}
	return namespace, true
}
