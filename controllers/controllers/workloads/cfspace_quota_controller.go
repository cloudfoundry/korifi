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

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspacequotas,verbs=get;list;watch;create;patch;delete

type CFSpaceQuotaReconciler struct {
	client client.Client
}

func NewCFSpaceQuotaReconciler(
	client client.Client,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFSpaceQuota, *korifiv1alpha1.CFSpaceQuota] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFSpaceQuota, *korifiv1alpha1.CFSpaceQuota](log, client, &CFSpaceQuotaReconciler{
		client: client,
	})
}

func (r *CFSpaceQuotaReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFSpaceQuota{})
}

func (r *CFSpaceQuotaReconciler) ReconcileResource(ctx context.Context, cfSpaceQuota *korifiv1alpha1.CFSpaceQuota) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
