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
	"fmt"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
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

// CFServiceBindingReconciler reconciles a CFServiceBinding object
type CFServiceBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

const (
	BindingSecretAvailableCondition = "BindingSecretAvailable"
)

//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfservicebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfservicebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfservicebindings/finalizers,verbs=update

func (r *CFServiceBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	cfServiceBinding := new(servicesv1alpha1.CFServiceBinding)
	err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cfServiceBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "unable to fetch CFServiceBinding", req.Name, req.Namespace)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := new(workloadsv1alpha1.CFApp)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	originalCfServiceBinding := cfServiceBinding.DeepCopy()
	err = controllerutil.SetOwnerReference(cfApp, cfServiceBinding, r.Scheme)
	if err != nil {
		r.Log.Error(err, "Unable to set owner reference on CfServiceBinding")
		return ctrl.Result{}, err
	}

	err = r.Client.Patch(ctx, cfServiceBinding, client.MergeFrom(originalCfServiceBinding))
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error setting owner reference on the CFServiceBinding %s/%s", req.Namespace, cfServiceBinding.Name))
		return ctrl.Result{}, err
	}

	instance := new(servicesv1alpha1.CFServiceInstance)
	var errorReturn error
	var result ctrl.Result
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: req.Namespace}, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cfServiceBinding.Status.Binding.Name = ""
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "ServiceInstanceNotFound",
				Message: "Service instance does not exist",
			})
			result = ctrl.Result{RequeueAfter: 2 * time.Second}
			err = nil
		} else {
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "UnknownError",
				Message: "Error occurred while fetching service instance: " + err.Error(),
			})
			result = ctrl.Result{}
		}
		statusErr := r.setStatus(ctx, cfServiceBinding)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return result, err
	}

	secret := new(corev1.Secret)
	// Note: is there a reason to fetch the secret name from the service instance spec?
	err = r.Client.Get(ctx, types.NamespacedName{Name: instance.Spec.SecretName, Namespace: req.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cfServiceBinding.Status.Binding.Name = ""
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "SecretNotFound",
				Message: "Binding secret does not exist",
			})
			result = ctrl.Result{RequeueAfter: 2 * time.Second}
		} else {
			errorReturn = err
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "UnknownError",
				Message: "Error occurred while fetching secret: " + err.Error(),
			})
			result = ctrl.Result{}
		}
	} else {
		cfServiceBinding.Status.Binding.Name = instance.Spec.SecretName
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:    BindingSecretAvailableCondition,
			Status:  metav1.ConditionTrue,
			Reason:  "SecretFound",
			Message: "",
		})
		result = ctrl.Result{}
	}

	err = r.setStatus(ctx, cfServiceBinding)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, errorReturn
}

func (r *CFServiceBindingReconciler) setStatus(ctx context.Context, cfServiceBinding *servicesv1alpha1.CFServiceBinding) error {
	if statusErr := r.Client.Status().Update(ctx, cfServiceBinding); statusErr != nil {
		r.Log.Error(statusErr, "unable to update CFServiceBinding status")
		return statusErr
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicesv1alpha1.CFServiceBinding{}).
		Complete(r)
}
