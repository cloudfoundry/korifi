package k8s

import (
	"context"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func SetOwner(ctx context.Context, k8sClient client.Client, owner, owned client.Object) error {
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), owner)
	if err != nil {
		return err
	}

	return controllerutil.SetOwnerReference(owner, owned, scheme.Scheme)
}
