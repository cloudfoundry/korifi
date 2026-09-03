package runnerinfo

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type RunnerInfoReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewRunnerInfoReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.RunnerInfo] {
	r := &RunnerInfoReconciler{k8sClient: c, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.RunnerInfo](log, c, r)
}

func (r *RunnerInfoReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		Named("knative-runnerinfo").
		For(&korifiv1alpha1.RunnerInfo{}).
		WithEventFilter(predicate.NewPredicateFuncs(filterRunnerInfos))
}

func filterRunnerInfos(object client.Object) bool {
	runnerInfo, ok := object.(*korifiv1alpha1.RunnerInfo)
	if !ok {
		return true
	}
	return runnerInfo.Name == controllers.AppWorkloadReconcilerName
}

func (r *RunnerInfoReconciler) ReconcileResource(ctx context.Context, runnerInfo *korifiv1alpha1.RunnerInfo) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	if !runnerInfo.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	runnerInfo.Status.ObservedGeneration = runnerInfo.Generation
	log.V(1).Info("set observed generation", "generation", runnerInfo.Status.ObservedGeneration)

	// Knative creates a new Revision per AppWorkload change; treat as rolling-capable
	// so CF /v3/deployments remain usable.
	runnerInfo.Status.Capabilities = korifiv1alpha1.RunnerInfoCapabilities{
		RollingDeploy: true,
	}

	return ctrl.Result{}, nil
}
