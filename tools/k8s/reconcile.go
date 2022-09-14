package k8s

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectWithDeepCopy[T any] interface {
	*T

	client.Object
	DeepCopy() *T
}

type ObjectReconciler[T any, PT ObjectWithDeepCopy[T]] interface {
	ReconcileResource(ctx context.Context, obj PT) (ctrl.Result, error)
	SetupWithManager(mgr ctrl.Manager) *builder.Builder
}

type PatchingReconciler[T any, PT ObjectWithDeepCopy[T]] struct {
	log              logr.Logger
	k8sClient        client.Client
	objectReconciler ObjectReconciler[T, PT]
}

func NewPatchingReconciler[T any, PT ObjectWithDeepCopy[T]](log logr.Logger, k8sClient client.Client, objectReconciler ObjectReconciler[T, PT]) *PatchingReconciler[T, PT] {
	return &PatchingReconciler[T, PT]{
		log:              log.WithName("patching-reconciler"),
		k8sClient:        k8sClient,
		objectReconciler: objectReconciler,
	}
}

func (r *PatchingReconciler[T, PT]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", req.Namespace, "name", req.Name)

	obj := PT(new(T))
	err := r.k8sClient.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, fmt.Sprintf("unable to fetch %T", obj))
		return ctrl.Result{}, err
	}

	originalObj := obj.DeepCopy()

	result, delegateErr := r.objectReconciler.ReconcileResource(ctx, obj)
	// copy obj as next update will reset the status
	reconciledObjCopy := obj.DeepCopy()

	err = r.k8sClient.Patch(ctx, obj, client.MergeFrom(PT(originalObj)))

	if err != nil {
		log.Error(err, "patch main object failed")
		return ctrl.Result{}, err
	}

	err = r.k8sClient.Status().Patch(ctx, PT(reconciledObjCopy), client.MergeFrom(PT(originalObj)))
	if err != nil {
		log.Error(err, "patch object status failed")
		return ctrl.Result{}, err
	}

	return result, delegateErr
}

func (r *PatchingReconciler[T, PT]) SetupWithManager(mgr ctrl.Manager) error {
	return r.objectReconciler.SetupWithManager(mgr).Complete(r)
}
