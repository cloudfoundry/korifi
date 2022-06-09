package wiring

import (
	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/pdb"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/prometheus"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func LRPReconciler(logger lager.Logger, manager manager.Manager, config eirinictrl.ControllerConfig) error {
	logger = logger.Session("lrp-reconciler")

	lrpReconciler, err := createLRPReconciler(logger, manager.GetClient(), config, manager.GetScheme())
	if err != nil {
		return errors.Wrap(err, "Failed to create LRP reconciler")
	}

	err = builder.
		ControllerManagedBy(manager).
		For(&eiriniv1.LRP{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(lrpReconciler)

	return errors.Wrapf(err, "Failed to build LRP reconciler")
}

func createLRPReconciler(
	logger lager.Logger,
	controllerClient client.Client,
	cfg eirinictrl.ControllerConfig,
	scheme *runtime.Scheme,
) (*reconciler.LRP, error) {
	logger = logger.Session("lrp-reconciler")
	lrpToStatefulSetConverter := stset.NewLRPToStatefulSetConverter(
		cfg.ApplicationServiceAccount,
		cfg.RegistrySecretName,
		cfg.UnsafeAllowAutomountServiceAccountToken,
		cfg.AllowRunImageAsRoot,
		k8s.CreateLivenessProbe,
		k8s.CreateReadinessProbe,
	)

	pdbUpdater := pdb.NewUpdater(controllerClient)
	desirer := stset.NewDesirer(logger, lrpToStatefulSetConverter, pdbUpdater, controllerClient, scheme)
	updater := stset.NewUpdater(logger, controllerClient, pdbUpdater)

	decoratedDesirer, err := prometheus.NewLRPDesirerDecorator(desirer, metrics.Registry, clock.RealClock{})
	if err != nil {
		return nil, err
	}

	return reconciler.NewLRP(
		logger,
		controllerClient,
		decoratedDesirer,
		updater,
	), nil
}
