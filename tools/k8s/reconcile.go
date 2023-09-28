package k8s

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
		log:              log,
		k8sClient:        k8sClient,
		objectReconciler: objectReconciler,
	}
}

func (r *PatchingReconciler[T, PT]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", req.Namespace, "name", req.Name, "logID", uuid.NewString())
	ctx = logr.NewContext(ctx, log)

	obj := PT(new(T))
	err := r.k8sClient.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Info(fmt.Sprintf("unable to fetch %T", obj), "reason", err)
		return ctrl.Result{}, err
	}

	var (
		result      ctrl.Result
		delegateErr error
	)

	err = Patch(ctx, r.k8sClient, obj, func() {
		result, delegateErr = r.objectReconciler.ReconcileResource(ctx, obj)
	})
	if err != nil {
		log.Info("patch object failed", "reason", err)
		return ctrl.Result{}, err
	}

	return result, delegateErr
}

func (r *PatchingReconciler[T, PT]) SetupWithManager(mgr ctrl.Manager) error {
	return r.objectReconciler.SetupWithManager(mgr).Complete(r)
}
