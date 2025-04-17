package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func PatchResource[T client.Object](
	ctx context.Context,
	k8sClient client.Client,
	obj T,
	modify func(),
) error {
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return fmt.Errorf("failed to get %T %v: %w", obj, client.ObjectKeyFromObject(obj), err)
	}

	return errors.Wrapf(
		k8s.PatchResource(ctx, k8sClient, obj, modify),
		"failed to patch %T %v", obj, client.ObjectKeyFromObject(obj),
	)
}

func GetAndPatch(
	ctx context.Context,
	klient Klient,
	obj client.Object,
	modify func() error,
) error {
	err := klient.Get(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to get %T %v: %w", obj, client.ObjectKeyFromObject(obj), err)
	}

	return errors.Wrapf(
		klient.Patch(ctx, obj, modify),
		"failed to patch %T %v", obj, client.ObjectKeyFromObject(obj),
	)
}
