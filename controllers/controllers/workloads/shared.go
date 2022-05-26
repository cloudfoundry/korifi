package workloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	"context"
	"fmt"
	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name CFClient . CFClient
type CFClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	Status() client.StatusWriter
}

// This is a helper function for updating local copy of status conditions
func setStatusConditionOnLocalCopy(conditions *[]metav1.Condition, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})
}

func createOrPatchNamespace(ctx context.Context, client client.Client, log logr.Logger, object client.Object, labels map[string]string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: object.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, client, namespace, func() error {
		if namespace.ObjectMeta.Labels == nil {
			namespace.ObjectMeta.Labels = make(map[string]string)
		}

		for key, value := range labels {
			namespace.ObjectMeta.Labels[key] = value
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Namespace/%s %s", object.GetName(), result))
	return nil
}

func propagateSecrets(ctx context.Context, client client.Client, log logr.Logger, object client.Object, secretName string) error {
	secret := new(corev1.Secret)
	err := client.Get(ctx, types.NamespacedName{Namespace: object.GetNamespace(), Name: secretName}, secret)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error fetching secret  %s/%s", object.GetNamespace(), secretName))
		return err
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: object.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, client, newSecret, func() error {
		newSecret.ObjectMeta.Annotations = secret.Annotations
		newSecret.ObjectMeta.Labels = secret.Labels
		newSecret.Immutable = secret.Immutable
		newSecret.Data = secret.Data
		newSecret.StringData = secret.StringData
		newSecret.Type = secret.Type
		return nil
	})

	if err != nil {
		log.Error(err, fmt.Sprintf("Error creating/patching secret %s/%s", newSecret.Namespace, newSecret.Name))
		return err
	}

	log.Info(fmt.Sprintf("Secret %s/%s %s", newSecret.Namespace, newSecret.Name, result))
	return nil
}

func propagateRoles(ctx context.Context, kClient client.Client, log logr.Logger, object client.Object) error {
	roles := new(rbacv1.RoleList)
	err := kClient.List(ctx, roles, client.InNamespace(object.GetNamespace()))
	if err != nil {
		log.Error(err, fmt.Sprintf("Error listing roles from namespace %s", object.GetNamespace()))
		return err
	}

	for index, _ := range roles.Items {
		newRole := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roles.Items[index].Name,
				Namespace: object.GetName(),
			},
		}
		result, err := controllerutil.CreateOrPatch(ctx, kClient, newRole, func() error {
			newRole.ObjectMeta.Labels = roles.Items[index].Labels
			newRole.ObjectMeta.Annotations = roles.Items[index].Annotations
			newRole.Rules = roles.Items[index].Rules
			return nil
		})

		if err != nil {
			log.Error(err, fmt.Sprintf("Error creating/patching role  %s/%s", newRole.Namespace, newRole.Name))
			return err
		}

		log.Info(fmt.Sprintf("Role %s/%s %s", newRole.Namespace, newRole.Name, result))
	}

	return nil
}

func propagateRoleBindings(ctx context.Context, kClient client.Client, log logr.Logger, object client.Object) error {
	roleBindings := new(rbacv1.RoleBindingList)
	labelSelector, err := labels.Parse(repositories.PropagateCFRoleLabel + " notin (false)")
	if err != nil {
		return err
	}

	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     object.GetNamespace(),
	}
	err = kClient.List(ctx, roleBindings, &listOptions)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error listing role-bindings from namespace %s", object.GetName()))
		return err
	}

	for index, _ := range roleBindings.Items {
		newRoleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleBindings.Items[index].Name,
				Namespace: object.GetName(),
			},
		}
		result, err := controllerutil.CreateOrPatch(ctx, kClient, newRoleBinding, func() error {
			newRoleBinding.ObjectMeta.Labels = roleBindings.Items[index].Labels
			newRoleBinding.ObjectMeta.Annotations = roleBindings.Items[index].Annotations
			newRoleBinding.Subjects = roleBindings.Items[index].Subjects
			newRoleBinding.RoleRef = roleBindings.Items[index].RoleRef
			return nil
		})

		if err != nil {
			log.Error(err, fmt.Sprintf("Error creating/patching role-bindings %s/%s", newRoleBinding.Namespace, newRoleBinding.Name))
			return err
		}

		log.Info(fmt.Sprintf("Role Binding %s/%s %s", newRoleBinding.Namespace, newRoleBinding.Name, result))
	}

	return nil
}

func isFinalizing(object metav1.Object) bool {
	if object.GetDeletionTimestamp() != nil && !object.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

func finalize(ctx context.Context, kClient client.Client, log logr.Logger, object client.Object, finalizerName string) (ctrl.Result, error) {
	log.Info(fmt.Sprintf("Reconciling deletion of %s", object.GetName()))

	if !controllerutil.ContainsFinalizer(object, finalizerName) {
		return ctrl.Result{}, nil
	}

	err := kClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: object.GetName()}})
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to delete namespace %s/%s", object.GetNamespace(), object.GetName()))
		return ctrl.Result{}, err
	}

	originalCFObject := object.DeepCopyObject().(client.Object)
	controllerutil.RemoveFinalizer(object, finalizerName)

	if err = kClient.Patch(ctx, object, client.MergeFrom(originalCFObject)); err != nil {
		log.Error(err, fmt.Sprintf("Failed to remove finalizer on CFSpace %s/%s", object.GetNamespace(), object.GetName()))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func updateStatus(ctx context.Context, client client.Client, object client.Object, conditionStatus metav1.ConditionStatus) error {
	switch obj := object.(type) {
	case *korifiv1alpha1.CFOrg:
		cfOrg := new(korifiv1alpha1.CFOrg)
		obj.DeepCopyInto(cfOrg)
		setStatusConditionOnLocalCopy(&cfOrg.Status.Conditions, StatusConditionReady, conditionStatus, StatusConditionReady, "")
		err := client.Status().Update(ctx, cfOrg)
		return err
	case *korifiv1alpha1.CFSpace:
		cfSpace := new(korifiv1alpha1.CFSpace)
		obj.DeepCopyInto(cfSpace)
		setStatusConditionOnLocalCopy(&cfSpace.Status.Conditions, StatusConditionReady, conditionStatus, StatusConditionReady, "")
		err := client.Status().Update(ctx, cfSpace)
		return err
	default:
		return fmt.Errorf("unknown object passed to updateStatus function")
	}
}

func getNamespace(ctx context.Context, client client.Client, namespaceName string) (*corev1.Namespace, bool) {
	namespace := new(corev1.Namespace)
	err := client.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
	if err != nil {
		return nil, false
	}
	return namespace, true
}
