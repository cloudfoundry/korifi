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

package services

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
)

// CFServiceInstanceReconciler reconciles a CFServiceInstance object
type CFServiceInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfserviceinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfserviceinstances/finalizers,verbs=update

func (r *CFServiceInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	result := ctrl.Result{}

	cfServiceInstance := new(servicesv1alpha1.CFServiceInstance)
	err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cfServiceInstance)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "unable to fetch CFServiceInstance", req.Name, req.Namespace)
		}
		return result, client.IgnoreNotFound(err)
	}

	secret := new(corev1.Secret)
	var errorReturn error
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfServiceInstance.Spec.SecretName, Namespace: req.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cfServiceInstance.Status.Binding.Name = ""
			meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "SecretNotFound",
				Message: "Binding secret does not exist",
			})
			result = ctrl.Result{RequeueAfter: 2 * time.Second}
		} else {
			errorReturn = err
			meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "UnknownError",
				Message: "Error occurred while fetching secret: " + err.Error(),
			})
		}
	} else {
		cfServiceInstance.Status.Binding.Name = cfServiceInstance.Spec.SecretName
		meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
			Type:    BindingSecretAvailableCondition,
			Status:  metav1.ConditionTrue,
			Reason:  "SecretFound",
			Message: "",
		})
	}
	if statusErr := r.Client.Status().Update(ctx, cfServiceInstance); statusErr != nil {
		r.Log.Error(statusErr, "unable to update CFServiceInstance status")
		return ctrl.Result{}, statusErr
	}
	return result, errorReturn
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicesv1alpha1.CFServiceInstance{}).
		Complete(r)
}
