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
//
// Note that this function should be used when current user has permissions to
// patch both object's spec and status, e.g. in controllers context
func Patch[T any, PT ObjectWithDeepCopy[T]](
	ctx context.Context,
	k8sClient client.Client,
	obj PT,
	modify func(),
) error {
	objCopy := PT(obj.DeepCopy())

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

	err = k8sClient.Patch(ctx, obj, client.MergeFrom(objCopy))
	if err != nil {
		return err
	}

	if objHasStatus {
		err = k8sClient.Status().Patch(ctx, modifiedObj, client.MergeFrom(objCopy))
		if err != nil {
			return err
		}

		// Now that we have patched the status using the intermediate object
		// copy, we need to set it onto the original
		return copyInto(modifiedObj, obj)
	}

	return nil
}

// PatchResource updates k8s objects by calling k8s client `Patch`. It does not
// patch the object status which makes it convenient to use in contexts where
// the current user is not permitted to patch the status, such as within the
// api repositories. The `modify` lambda is expected to mutate the `obj` but
// does not take the object as an argument as the object should be visible in
// the parent scope Example:
//
//	var pod *corev1.Pod
//
//	patchErr = k8s.PatchResource(ctx, fakeClient, pod, func() {
//	  pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
//	})
//
// Note that this function should be used when current user has permissions to
// patch both object's spec and status, e.g. in controllers context
func PatchResource[T any, PT ObjectWithDeepCopy[T]](
	ctx context.Context,
	k8sClient client.Client,
	obj PT,
	modify func(),
) error {
	originalObj := PT(obj.DeepCopy())

	modify()

	return k8sClient.Patch(ctx, obj, client.MergeFrom(originalObj))
}

func hasStatus(obj runtime.Object) (bool, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return false, err
	}

	_, hasStatusField := unstructuredObj["status"]
	return hasStatusField, nil
}

func copyInto(sourceObj, targetObj runtime.Object) error {
	unstructuredSource, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sourceObj)
	if err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredSource, targetObj)
}
