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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CFPackageReconciler reconciles a CFPackage object
type CFPackageReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewCFPackageReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) *k8s.PatchingReconciler[korifiv1alpha1.CFPackage, *korifiv1alpha1.CFPackage] {
	pkgReconciler := CFPackageReconciler{k8sClient: client, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFPackage, *korifiv1alpha1.CFPackage](log, client, &pkgReconciler)
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFPackage object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CFPackageReconciler) ReconcileResource(ctx context.Context, cfPackage *korifiv1alpha1.CFPackage) (ctrl.Result, error) {
	var cfApp korifiv1alpha1.CFApp
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfPackage.Spec.AppRef.Name, Namespace: cfPackage.Namespace}, &cfApp)
	if err != nil {
		r.log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(&cfApp, cfPackage, r.scheme)
	if err != nil {
		r.log.Info("unable to set owner reference on CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFPackageReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFPackage{})
}
