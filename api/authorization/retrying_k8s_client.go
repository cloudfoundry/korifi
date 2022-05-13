package authorization

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/correlation"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDefaultBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: 5 * time.Millisecond,
		Factor:   2,
		Steps:    10,
	}
}

type AuthRetryingClient struct {
	client.WithWatch
	backoff wait.Backoff
	logger  logr.Logger
}

func NewAuthRetryingClient(c client.WithWatch, backoff wait.Backoff) client.WithWatch {
	return AuthRetryingClient{
		WithWatch: c,
		backoff:   backoff,
		logger:    ctrl.Log.WithName("retrying-client"),
	}
}

func (a AuthRetryingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.Get(ctx, key, obj)
	}, "get")
}

func (a AuthRetryingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.List(ctx, list, opts...)
	}, "list")
}

func (a AuthRetryingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.Create(ctx, obj, opts...)
	}, "create")
}

func (a AuthRetryingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.Delete(ctx, obj, opts...)
	}, "delete")
}

func (a AuthRetryingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.Update(ctx, obj, opts...)
	}, "update")
}

func (a AuthRetryingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.Patch(ctx, obj, patch, opts...)
	}, "patch")
}

func (a AuthRetryingClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return a.retryOnForbidden(ctx, func() error {
		return a.WithWatch.DeleteAllOf(ctx, obj, opts...)
	}, "deleteAllOf")
}

func (a AuthRetryingClient) retryOnForbidden(ctx context.Context, fn func() error, op string) error {
	count := 0
	return retry.OnError(a.backoff, k8serrors.IsForbidden, func() error {
		logger := correlation.AddCorrelationIDToLogger(ctx, a.logger)
		err := fn()
		if err != nil {
			count++
			logger.Info("k8s client returned forbidden", "op", op, "count", count, "error", err)
		}
		return err
	})
}
