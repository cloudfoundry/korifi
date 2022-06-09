package wiring

import (
	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/jobs"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TaskReconciler(logger lager.Logger, manager manager.Manager, config eirinictrl.ControllerConfig) error {
	logger = logger.Session("task-reconciler")

	taskReconciler := createTaskReconciler(logger, manager.GetClient(), config, manager.GetScheme())

	err := builder.
		ControllerManagedBy(manager).
		For(&eiriniv1.Task{}).
		Owns(&batchv1.Job{}).
		Complete(taskReconciler)

	return errors.Wrapf(err, "Failed to build Task reconciler")
}

func createTaskReconciler(
	logger lager.Logger,
	controllerClient client.Client,
	cfg eirinictrl.ControllerConfig,
	scheme *runtime.Scheme,
) *reconciler.Task {
	taskToJobConverter := jobs.NewTaskToJobConverter(
		cfg.ApplicationServiceAccount,
		cfg.RegistrySecretName,
		cfg.UnsafeAllowAutomountServiceAccountToken,
	)

	desirer := jobs.NewDesirer(logger, taskToJobConverter, controllerClient, scheme)
	statusGetter := jobs.NewStatusGetter(logger)

	return reconciler.NewTask(logger, controllerClient, desirer, statusGetter, cfg.TaskTTLSeconds)
}
