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

package networking

import (
	"context"
	"fmt"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	"github.com/go-logr/logr"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TODO: This seems too specific to the current implementation
	ProxyCreatedConditionType = "ProxyCreated"
)

// CFRouteReconciler reconciles a CFRoute object
type CFRouteReconciler struct {
	Client CFClient
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies/status,verbs=get
//+kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFRoute object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CFRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cfRoute networkingv1alpha1.CFRoute
	err := r.Client.Get(ctx, req.NamespacedName, &cfRoute)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFRoute")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var cfDomain networkingv1alpha1.CFDomain
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfRoute.Spec.DomainRef.Name}, &cfDomain)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFDomain")
		return ctrl.Result{}, err
	}

	proxyCreatedStatus := getConditionOrSetAsUnknown(&cfRoute.Status.Conditions, ProxyCreatedConditionType)

	if proxyCreatedStatus == metav1.ConditionUnknown {
		err = r.createContourHTTPProxyAndUpdateStatus(ctx, &cfRoute, &cfDomain)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// getConditionOrSetAsUnknown is a helper function that retrieves the value of the provided conditionType, like "Succeeded" and returns the value: "True", "False", or "Unknown"
// If the value is not present, the pointer to the list of conditions provided to the function is used to add an entry to the list of Conditions with a value of "Unknown" and "Unknown" is returned
func getConditionOrSetAsUnknown(conditions *[]metav1.Condition, conditionType string) metav1.ConditionStatus {
	conditionStatus := meta.FindStatusCondition(*conditions, conditionType)
	conditionStatusValue := metav1.ConditionUnknown
	if conditionStatus != nil {
		conditionStatusValue = conditionStatus.Status
	} else {
		// set local copy of CR condition to "unknown" because it had no value
		meta.SetStatusCondition(conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "Unknown", // TODO: Think about this. Consumers of status will care?
			Message: "Unknown",
		})
	}

	return conditionStatusValue
}

func (r *CFRouteReconciler) createContourHTTPProxyAndUpdateStatus(ctx context.Context, cfRoute *networkingv1alpha1.CFRoute, cfDomain *networkingv1alpha1.CFDomain) error {
	desiredContourHTTPProxy := contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRoute.Name,
			Namespace: cfRoute.Namespace,
			Labels: map[string]string{
				networkingv1alpha1.CFRouteGUIDLabelKey:  cfRoute.Name,
				networkingv1alpha1.CFDomainGUIDLabelKey: cfDomain.Name,
			},
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name),
			},
		},
	}

	err := r.createContourHTTPProxyIfNotExists(ctx, desiredContourHTTPProxy)
	if err != nil {
		return err
	}

	// Update and set "ProxyCreated" status to True
	meta.SetStatusCondition(&cfRoute.Status.Conditions, metav1.Condition{
		Type:    ProxyCreatedConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "ContourResourceCreate",
		Message: "Successfully created HTTPProxy",
	})

	// Update Route Status Conditions based on changes made to local copy
	if err := r.Client.Status().Update(ctx, cfRoute); err != nil {
		r.Log.Error(err, "Error when updating CFRoute status")
		return err
	}

	return nil
}

func (r *CFRouteReconciler) createContourHTTPProxyIfNotExists(ctx context.Context, desiredContourHTTPProxy contourv1.HTTPProxy) error {
	foundContourHTTPProxy := contourv1.HTTPProxy{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredContourHTTPProxy.Name, Namespace: desiredContourHTTPProxy.Namespace}, &foundContourHTTPProxy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.Client.Create(ctx, &desiredContourHTTPProxy)
			if err != nil {
				r.Log.Error(err, "Error when creating Contour HTTPProxy")
				return err
			}
		} else {
			r.Log.Error(err, "Error when checking if Contour HTTPProxy exists")
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha1.CFRoute{}).
		Complete(r)
}
