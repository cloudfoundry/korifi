package k8sns

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Finalizer[T any, NS NamespaceObject[T]] interface {
	Finalize(ctx context.Context, obj NS) (ctrl.Result, error)
}

type NoopFinalizer[T any, NS NamespaceObject[T]] struct{}

func (f *NoopFinalizer[T, NS]) Finalize(ctx context.Context, obj NS) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

type NamespaceFinalizer[T any, NS NamespaceObject[T]] struct {
	client            client.Client
	delegateFinalizer Finalizer[T, NS]
	finalizerName     string
}

func NewNamespaceFinalizer[T any, NS NamespaceObject[T]](
	client client.Client,
	delegateFinalizer Finalizer[T, NS],
	finalizerName string,
) *NamespaceFinalizer[T, NS] {
	return &NamespaceFinalizer[T, NS]{
		client:            client,
		delegateFinalizer: delegateFinalizer,
		finalizerName:     finalizerName,
	}
}

func (f *NamespaceFinalizer[T, NS]) Finalize(ctx context.Context, obj NS) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalize")

	if !controllerutil.ContainsFinalizer(obj, f.finalizerName) {
		return ctrl.Result{}, nil
	}

	delegateResult, err := f.delegateFinalizer.Finalize(ctx, obj)
	if (delegateResult != ctrl.Result{}) || (err != nil) {
		return delegateResult, err
	}

	err = f.client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: obj.GetName()}})
	if k8serrors.IsNotFound(err) {
		if controllerutil.RemoveFinalizer(obj, f.finalizerName) {
			log.V(1).Info("finalizer removed")
		}

		return ctrl.Result{}, nil
	}

	if err != nil {
		log.Info("failed to delete namespace", "reason", err)
		return ctrl.Result{}, err
	}

	log.V(1).Info("requeuing waiting for namespace deletion")

	return ctrl.Result{RequeueAfter: time.Second}, nil
}
