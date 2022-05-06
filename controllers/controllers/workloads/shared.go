package workloads

import (
	"context"
	"fmt"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client

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

func updateStatus(ctx context.Context, client client.Client, object client.Object, conditionStatus metav1.ConditionStatus) error {
	switch obj := object.(type) {
	case *workloadsv1alpha1.CFOrg:
		cfOrg := new(workloadsv1alpha1.CFOrg)
		obj.DeepCopyInto(cfOrg)
		setStatusConditionOnLocalCopy(&cfOrg.Status.Conditions, StatusConditionReady, conditionStatus, StatusConditionReady, "")
		err := client.Status().Update(ctx, cfOrg)
		return err
	case *workloadsv1alpha1.CFSpace:
		cfSpace := new(workloadsv1alpha1.CFSpace)
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

//counterfeiter:generate -o fake -fake-name StatusWriter sigs.k8s.io/controller-runtime/pkg/client.StatusWriter
