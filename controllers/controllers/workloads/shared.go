package workloads

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

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

func createSubnamespaceAnchor(ctx context.Context, client client.Client, req ctrl.Request, object client.Object, labels map[string]string) (v1alpha2.SubnamespaceAnchor, error) {
	anchor := v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: APIVersion,
					Kind:       object.GetObjectKind().GroupVersionKind().Kind,
					Name:       object.GetName(),
					UID:        object.GetUID(),
				},
			},
		},
	}

	err := client.Create(ctx, &anchor)
	if err != nil {
		return anchor, err
	}

	return anchor, nil
}

func updateStatus(ctx context.Context, client client.Client, object client.Object, conditionStatus metav1.ConditionStatus) error {
	switch obj := object.(type) {
	case *v1alpha1.CFOrg:
		cfOrg := new(v1alpha1.CFOrg)
		obj.DeepCopyInto(cfOrg)
		setStatusConditionOnLocalCopy(&cfOrg.Status.Conditions, StatusConditionReady, conditionStatus, StatusConditionReady, "")
		err := client.Status().Update(ctx, cfOrg)
		return err
	case *v1alpha1.CFSpace:
		cfSpace := new(v1alpha1.CFSpace)
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
