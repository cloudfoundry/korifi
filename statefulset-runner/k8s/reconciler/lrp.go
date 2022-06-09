package reconciler

import (
	"context"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/lager"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//counterfeiter:generate . LRPDesirer
//counterfeiter:generate . LRPUpdater

type LRPDesirer interface {
	Desire(ctx context.Context, lrp *eiriniv1.LRP) error
}

type LRPUpdater interface {
	Update(ctx context.Context, lrp *eiriniv1.LRP, stSet *appsv1.StatefulSet) error
}

func NewLRP(logger lager.Logger, client client.Client, desirer LRPDesirer, updater LRPUpdater) *LRP {
	return &LRP{
		logger:  logger,
		client:  client,
		desirer: desirer,
		updater: updater,
	}
}

type LRP struct {
	logger  lager.Logger
	client  client.Client
	desirer LRPDesirer
	updater LRPUpdater
}

func (r *LRP) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := r.logger.Session("reconcile-lrp",
		lager.Data{
			"name":      request.NamespacedName.Name,
			"namespace": request.NamespacedName.Namespace,
		})

	lrp := &eiriniv1.LRP{}

	err := r.client.Get(ctx, request.NamespacedName, lrp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("lrp-not-found")

			return reconcile.Result{}, nil
		}

		logger.Error("failed-to-get-lrp", err)

		return reconcile.Result{}, errors.Wrap(err, "failed to get lrp")
	}

	err = r.do(ctx, lrp)
	if err != nil {
		logger.Error("failed-to-reconcile", err)
	}

	return reconcile.Result{}, err
}

func (r *LRP) do(ctx context.Context, lrp *eiriniv1.LRP) error {
	stSetName, err := utils.GetStatefulsetName(lrp)
	if err != nil {
		return errors.Wrapf(err, "failed to determine statefulset name for lrp {%s}%s", lrp.Namespace, lrp.Name)
	}

	stSet := &appsv1.StatefulSet{}

	err = r.client.Get(ctx, client.ObjectKey{Namespace: lrp.Namespace, Name: stSetName}, stSet)
	if apierrors.IsNotFound(err) {
		desireErr := r.desirer.Desire(ctx, lrp)

		return errors.Wrap(desireErr, "failed to desire lrp")
	}

	if err != nil {
		return errors.Wrap(err, "failed to get statefulSet")
	}

	var errs *multierror.Error

	err = r.updateLRPStatus(ctx, lrp, stSet)
	errs = multierror.Append(errs, errors.Wrap(err, "failed to update lrp status"))

	err = r.updater.Update(ctx, lrp, stSet)
	errs = multierror.Append(errs, errors.Wrap(err, "failed to update app"))

	return errs.ErrorOrNil()
}

func (r *LRP) updateLRPStatus(ctx context.Context, lrp *eiriniv1.LRP, stSet *appsv1.StatefulSet) error {
	originalLRP := lrp.DeepCopy()
	lrp.Status.Replicas = stSet.Status.ReadyReplicas

	return r.client.Status().Patch(ctx, lrp, client.MergeFrom(originalLRP))
}
