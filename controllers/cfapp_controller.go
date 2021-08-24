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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/client-interfaces"
)

// CFAppReconciler reconciles a CFApp object
type CFAppReconciler struct {
	Client CFAppClient
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps/finalizers,verbs=update


// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CFAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cfApp workloadsv1alpha1.CFApp
	err := r.Client.Get(ctx, req.NamespacedName, &cfApp)
	//cfApp, err := r.CFAppClient.Get(ctx, req.NamespacedName)
	if err != nil {
		r.Log.Error(err, "unable to fetch CFApp")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// set the status.conditions "Running" and "Restarting" to false
	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    "Running",
		Status:  metav1.ConditionFalse,
		Reason:  "unimplemented",
		Message: "",
	})
	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    "Restarting",
		Status:  metav1.ConditionFalse,
		Reason:  "unimplemented",
		Message: "",
	})

	// Update CF App Status Conditions based on local copy
	if err := r.Client.Status().Update(ctx, &cfApp); err != nil {
		r.Log.Error(err, "unable to update CFApp status")
		r.Log.Info(fmt.Sprintf("CFApps status: %+v", cfApp.Status))
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFApp{}).
		Complete(r)
}
