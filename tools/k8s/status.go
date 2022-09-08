package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RuntimeObjectWithDeepCopy[T any] interface {
	client.Object
	StatusConditions() *[]metav1.Condition
	DeepCopy() T
}

func PatchStatus[T RuntimeObjectWithDeepCopy[T]](
	ctx context.Context,
	k8sClient client.Client,
	obj T,
	modifyStatus func(),
	conditions ...metav1.Condition,
) error {
	originalObj := obj.DeepCopy()
	modifyStatus()
	for _, condition := range conditions {
		meta.SetStatusCondition(obj.StatusConditions(), condition)
	}
	return k8sClient.Status().Patch(ctx, obj, client.MergeFrom(originalObj))
}

func PatchStatusConditions[T RuntimeObjectWithDeepCopy[T]](
	ctx context.Context,
	k8sClient client.Client,
	obj T,
	conditions ...metav1.Condition,
) error {
	return PatchStatus(ctx, k8sClient, obj, func() {}, conditions...)
}
