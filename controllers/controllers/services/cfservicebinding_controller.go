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

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CFServiceBindingReconciler reconciles a CFServiceBinding object
type CFServiceBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

const (
	BindingSecretAvailableCondition   = "BindingSecretAvailable"
	ServiceBindingGUIDLabel           = "korifi.cloudfoundry.org/service-binding-guid"
	ServiceCredentialBindingTypeLabel = "korifi.cloudfoundry.org/service-credential-binding-type"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/finalizers,verbs=update
//+kubebuilder:rbac:groups=servicebinding.io,resources=servicebindings,verbs=get;list;create;update;patch;watch

func (r *CFServiceBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	cfServiceBinding := new(v1alpha1.CFServiceBinding)
	err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cfServiceBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "unable to fetch CFServiceBinding", req.Name, req.Namespace)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := new(v1alpha1.CFApp)
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

	instance := new(v1alpha1.CFServiceInstance)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: req.Namespace}, instance)
	if err != nil {
		var result ctrl.Result
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
		var result ctrl.Result
		if apierrors.IsNotFound(err) {
			cfServiceBinding.Status.Binding.Name = ""
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "SecretNotFound",
				Message: "Binding secret does not exist",
			})
			result = ctrl.Result{RequeueAfter: 2 * time.Second}
			err = nil
		} else {
			meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
				Type:    BindingSecretAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "UnknownError",
				Message: "Error occurred while fetching secret: " + err.Error(),
			})
			result = ctrl.Result{}
		}
		statusErr := r.setStatus(ctx, cfServiceBinding)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return result, err
	} else {
		cfServiceBinding.Status.Binding.Name = instance.Spec.SecretName
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:    BindingSecretAvailableCondition,
			Status:  metav1.ConditionTrue,
			Reason:  "SecretFound",
			Message: "",
		})

		err = r.setStatus(ctx, cfServiceBinding)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	actualSBServiceBinding := servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
		},
	}

	desiredSBServiceBinding := generateDesiredServiceBinding(&actualSBServiceBinding, cfServiceBinding, cfApp, secret)

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, &actualSBServiceBinding, sbServiceBindingMutateFn(&actualSBServiceBinding, desiredSBServiceBinding))
	if err != nil {
		r.Log.Error(err, "Error calling Create on servicebinding.io ServiceBinding")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func sbServiceBindingMutateFn(actualSBServiceBinding, desiredSBServiceBinding *servicebindingv1beta1.ServiceBinding) controllerutil.MutateFn {
	return func() error {
		actualSBServiceBinding.ObjectMeta.Labels = desiredSBServiceBinding.ObjectMeta.Labels
		actualSBServiceBinding.ObjectMeta.OwnerReferences = desiredSBServiceBinding.ObjectMeta.OwnerReferences
		actualSBServiceBinding.Spec = desiredSBServiceBinding.Spec
		return nil
	}
}

func generateDesiredServiceBinding(actualServiceBinding *servicebindingv1beta1.ServiceBinding, cfServiceBinding *v1alpha1.CFServiceBinding, cfApp *v1alpha1.CFApp, secret *corev1.Secret) *servicebindingv1beta1.ServiceBinding {
	var desiredServiceBinding servicebindingv1beta1.ServiceBinding
	actualServiceBinding.DeepCopyInto(&desiredServiceBinding)
	desiredServiceBinding.ObjectMeta.Labels = map[string]string{
		ServiceBindingGUIDLabel:           cfServiceBinding.Name,
		v1alpha1.CFAppGUIDLabelKey:        cfApp.Name,
		ServiceCredentialBindingTypeLabel: "app",
	}
	desiredServiceBinding.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "korifi.cloudfoundry.org/v1alpha1",
			Kind:       "CFServiceBinding",
			Name:       cfServiceBinding.Name,
			UID:        cfServiceBinding.UID,
		},
	}
	desiredServiceBinding.Spec = servicebindingv1beta1.ServiceBindingSpec{
		Name: secret.Name,
		Type: "user-provided",
		Workload: servicebindingv1beta1.ServiceBindingWorkloadReference{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					v1alpha1.CFAppGUIDLabelKey: cfApp.Name,
				},
			},
		},
		Service: servicebindingv1beta1.ServiceBindingServiceReference{
			APIVersion: "korifi.cloudfoundry.org/v1alpha1",
			Kind:       "CFServiceBinding",
			Name:       cfServiceBinding.Name,
		},
	}
	secretType, ok := secret.Data["type"]
	if ok && len(secretType) > 0 {
		desiredServiceBinding.Spec.Type = string(secretType)
	}
	secretProvider, ok := secret.Data["provider"]
	if ok {
		desiredServiceBinding.Spec.Provider = string(secretProvider)
	}
	return &desiredServiceBinding
}

func (r *CFServiceBindingReconciler) setStatus(ctx context.Context, cfServiceBinding *v1alpha1.CFServiceBinding) error {
	if statusErr := r.Client.Status().Update(ctx, cfServiceBinding); statusErr != nil {
		r.Log.Error(statusErr, "unable to update CFServiceBinding status")
		return statusErr
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CFServiceBinding{}).
		Complete(r)
}
