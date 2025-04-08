package k8s

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RuntimeObjectWithStatusConditions interface {
	client.Object
	StatusConditions() *[]metav1.Condition
}

type ObjectReconciler[T any] interface {
	ReconcileResource(ctx context.Context, obj T) (ctrl.Result, error)
	SetupWithManager(mgr ctrl.Manager) *builder.Builder
}

type PatchingReconciler[T any] struct {
	log              logr.Logger
	k8sClient        client.Client
	objectReconciler ObjectReconciler[*T]
}

func NewPatchingReconciler[T any](log logr.Logger, k8sClient client.Client, objectReconciler ObjectReconciler[*T]) *PatchingReconciler[T] {
	return &PatchingReconciler[T]{
		log:              log,
		k8sClient:        k8sClient,
		objectReconciler: objectReconciler,
	}
}

func (r *PatchingReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.
		WithName(reflect.TypeFor[T]().Name()).
		WithValues("namespace", req.Namespace, "name", req.Name, "logID", uuid.NewString())
	ctx = logr.NewContext(ctx, log)

	runtimeObj := any(new(T)).(RuntimeObjectWithStatusConditions)

	err := r.k8sClient.Get(ctx, req.NamespacedName, runtimeObj)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Info(fmt.Sprintf("unable to fetch %T", runtimeObj), "reason", err)
		return ctrl.Result{}, err
	}

	var (
		result      ctrl.Result
		delegateErr error
	)

	err = Patch(ctx, r.k8sClient, runtimeObj, func() {
		readyConditionBuilder := NewReadyConditionBuilder(runtimeObj)
		defer func() {
			meta.SetStatusCondition(runtimeObj.StatusConditions(), readyConditionBuilder.WithError(delegateErr).Build())
		}()

		result, delegateErr = r.objectReconciler.ReconcileResource(ctx, any(runtimeObj).(*T))
		if delegateErr == nil {
			readyConditionBuilder.Ready()
			return
		}

		var notReadyErr NotReadyError
		if errors.As(delegateErr, &notReadyErr) {
			reason := notReadyErr.reason
			if reason == "" {
				reason = "Unknown"
			}

			readyConditionBuilder.WithReason(reason).WithMessage(notReadyErr.message)

			if notReadyErr.noRequeue {
				result = ctrl.Result{}
				delegateErr = nil
			}

			if notReadyErr.requeueAfter != nil {
				result = ctrl.Result{RequeueAfter: *notReadyErr.requeueAfter}
				delegateErr = nil
			}

			if notReadyErr.requeue {
				result = ctrl.Result{Requeue: true}
				delegateErr = nil
			}
		}
	})
	if err != nil {
		log.Info("patch object failed", "reason", err)
		return ctrl.Result{}, err
	}

	return result, delegateErr
}

func (r *PatchingReconciler[T]) SetupWithManager(mgr ctrl.Manager) error {
	return r.objectReconciler.SetupWithManager(mgr).Complete(r)
}

type NotReadyError struct {
	cause        error
	reason       string
	message      string
	requeueAfter *time.Duration
	requeue      bool
	noRequeue    bool
}

func (e NotReadyError) Error() string {
	if e.cause == nil {
		return e.message
	}

	message := e.message
	if message != "" {
		message = message + ": "
	}

	return fmt.Sprintf("%s%s", message, e.cause.Error())
}

func NewNotReadyError() NotReadyError {
	return NotReadyError{}
}

func (e NotReadyError) WithCause(cause error) NotReadyError {
	e.cause = cause
	return e
}

func (e NotReadyError) WithRequeue() NotReadyError {
	e.requeue = true
	return e
}

func (e NotReadyError) WithRequeueAfter(duration time.Duration) NotReadyError {
	e.requeueAfter = tools.PtrTo(duration)
	return e
}

func (e NotReadyError) WithNoRequeue() NotReadyError {
	e.noRequeue = true
	return e
}

func (e NotReadyError) WithReason(reason string) NotReadyError {
	e.reason = reason
	return e
}

func (e NotReadyError) WithMessage(message string) NotReadyError {
	e.message = message
	return e
}
