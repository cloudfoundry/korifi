package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectWithDeepCopy[T any] interface {
	*T

	client.Object
	DeepCopy() *T
}

// Patch updates k8s objects by subsequently calling k8s client `Patch()` and
// `Status().Patch()` The `modify` lambda is expected to mutate the `obj` but
// does not take the object as an argument as the object should be visible in
// the parent scope
// Example:
//
//	var pod *corev1.Pod
//
//	patchErr = k8s.Patch(ctx, fakeClient, pod, func() {
//		pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
//		pod.Status.Message = "hello"
//	})
func Patch[T any, PT ObjectWithDeepCopy[T]](
	ctx context.Context,
	k8sClient client.Client,
	obj PT,
	modify func(),
) error {
	originalObj := PT(obj.DeepCopy())

	// modify func takes no args, because it is a lambda that sees obj from the parent scope, e.g
	// Patch(ctx, k8sClient
	modify()

	// Deep copy the original object after the modification is performed so
	// that we capture status modifications We need to do that because the
	// object patch below modifies the obj parameter to reflect the state in
	// etcd, i.e. clears all modifications on the status
	modifiedObj := PT(obj.DeepCopy())

	objHasStatus, err := hasStatus(obj)
	if err != nil {
		return err
	}

	err = k8sClient.Patch(ctx, obj, client.MergeFrom(originalObj))
	if err != nil {
		return err
	}

	if objHasStatus {
		return k8sClient.Status().Patch(ctx, modifiedObj, client.MergeFrom(originalObj))
	}

	return nil
}

func hasStatus(obj runtime.Object) (bool, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return false, err
	}

	_, hasStatusField := unstructuredObj["status"]
	return hasStatusField, nil
}
