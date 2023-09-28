/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// RunnerInfoReconciler reconciles a RunnerInfo object
type RunnerInfoReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewRunnerInfoReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.RunnerInfo, *korifiv1alpha1.RunnerInfo] {
	runnerInfoReconciler := RunnerInfoReconciler{
		k8sClient: c,
		scheme:    scheme,
		log:       log,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.RunnerInfo, *korifiv1alpha1.RunnerInfo](log, c, &runnerInfoReconciler)
}

func (r *RunnerInfoReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.RunnerInfo{}).
		WithEventFilter(predicate.NewPredicateFuncs(filterRunnerInfos))
}

func filterRunnerInfos(object client.Object) bool {
	runnerInfo, ok := object.(*korifiv1alpha1.RunnerInfo)
	if !ok {
		return true
	}

	return runnerInfo.Name == AppWorkloadReconcilerName
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=runnerinfos,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=runnerinfos/status,verbs=get;patch

func (r *RunnerInfoReconciler) ReconcileResource(ctx context.Context, runnerInfo *korifiv1alpha1.RunnerInfo) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	runnerInfo.Status.ObservedGeneration = runnerInfo.Generation
	log.V(1).Info("set observed generation", "generation", runnerInfo.Status.ObservedGeneration)

	runnerInfo.Status.Capabilities = korifiv1alpha1.RunnerInfoCapabilities{
		RollingDeploy: true,
	}

	return ctrl.Result{}, nil
}
