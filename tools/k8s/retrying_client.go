package k8s

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDefaultBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: 5 * time.Millisecond,
		Factor:   2,
		Steps:    10,
	}
}

type RetryingClient struct {
	client.WithWatch
	predicate func(error) bool
	backoff   wait.Backoff
}

func NewRetryingClient(c client.WithWatch, predicate func(error) bool, backoff wait.Backoff) client.WithWatch {
	return RetryingClient{
		WithWatch: c,
		predicate: predicate,
		backoff:   backoff,
	}
}

func (a RetryingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.Get(ctx, key, obj, opts...)
	}, "get")
}

func (a RetryingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.List(ctx, list, opts...)
	}, "list")
}

func (a RetryingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.Create(ctx, obj, opts...)
	}, "create")
}

func (a RetryingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.Delete(ctx, obj, opts...)
	}, "delete")
}

func (a RetryingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.Update(ctx, obj, opts...)
	}, "update")
}

func (a RetryingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.Patch(ctx, obj, patch, opts...)
	}, "patch")
}

func (a RetryingClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return a.retryOnError(ctx, func() error {
		return a.WithWatch.DeleteAllOf(ctx, obj, opts...)
	}, "deleteAllOf")
}

func (a RetryingClient) retryOnError(ctx context.Context, fn func() error, op string) error {
	count := 0
	return retry.OnError(a.backoff, a.predicate, func() error {
		logger := logr.FromContextOrDiscard(ctx).WithName("retrying-client")
		err := fn()
		if err != nil {
			count++
			logger.Info("k8s client returned error", "op", op, "count", count, "error", err)
		}
		return err
	})
}
