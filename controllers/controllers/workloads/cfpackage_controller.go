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
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CFPackageReconciler reconciles a CFPackage object
type CFPackageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfpackages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfpackages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfpackages/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFPackage object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CFPackageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cfPackage workloadsv1alpha1.CFPackage
	err := r.Client.Get(ctx, req.NamespacedName, &cfPackage)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFPackage")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var cfApp workloadsv1alpha1.CFApp
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfPackage.Spec.AppRef.Name, Namespace: cfPackage.Namespace}, &cfApp)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	originalCFPackage := cfPackage.DeepCopy()
	err = controllerutil.SetOwnerReference(&cfApp, &cfPackage, r.Scheme)
	if err != nil {
		r.Log.Error(err, "unable to set owner reference on CFPackage")
		return ctrl.Result{}, err
	}

	err = r.Client.Patch(ctx, &cfPackage, client.MergeFrom(originalCFPackage))
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error setting owner reference on the CFPackage %s/%s", req.Namespace, cfPackage.Name))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFPackageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFPackage{}).
		Complete(r)
}
