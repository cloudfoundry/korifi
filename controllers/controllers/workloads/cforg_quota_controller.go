/*
Copyright 2021.

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

package workloads

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgquotas,verbs=get;list;watch;create;patch;delete

type CFOrgQuotaReconciler struct {
	client client.Client
}

func NewCFOrgQuotaReconciler(
	client client.Client,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFOrgQuota, *korifiv1alpha1.CFOrgQuota] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFOrgQuota, *korifiv1alpha1.CFOrgQuota](log, client, &CFOrgQuotaReconciler{
		client: client,
	})
}

func (r *CFOrgQuotaReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFOrgQuota{})
}

func (r *CFOrgQuotaReconciler) ReconcileResource(ctx context.Context, cfOrgQuota *korifiv1alpha1.CFOrgQuota) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
