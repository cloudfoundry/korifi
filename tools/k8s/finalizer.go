package k8s

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func AddFinalizer[T any, PT ObjectWithDeepCopy[T]](
	ctx context.Context,
	log logr.Logger,
	k8sClient client.Client,
	obj PT,
	finalizerName string,
) error {
	origObj := PT(obj.DeepCopy())
	if controllerutil.AddFinalizer(obj, finalizerName) {
		return k8sClient.Patch(ctx, obj, client.MergeFrom(origObj))
	}

	log.Info("Finalizer added")

	return nil
}
