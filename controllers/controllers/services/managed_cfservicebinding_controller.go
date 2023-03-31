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
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	btpv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ManagedCFServiceBindingReconciler reconciles a CFServiceBinding object
type ManagedCFServiceBindingReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewManagedCFServiceBindingReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding] {
	cfBindingReconciler := &ManagedCFServiceBindingReconciler{k8sClient: k8sClient, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding](log, k8sClient, cfBindingReconciler)
}

//+kubebuilder:rbac:groups=services.cloud.sap.com,resources=servicebindings,verbs=get;list;create;update;patch;watch

func (r *ManagedCFServiceBindingReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	instance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, instance)
	if err != nil {
		r.log.Error(err, "failed to get managed service instance")
		return ctrl.Result{}, err
	}

	err = controllerutil.SetOwnerReference(instance, cfServiceBinding, r.scheme)
	if err != nil {
		r.log.Error(err, "failed to set owner reference from managed service binding to service instance")
		return ctrl.Result{}, err
	}

	bindingSecretName := uuid.NewString()
	btpServiceBinding := &btpv1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Name,
		},
		Spec: btpv1.ServiceBindingSpec{
			ServiceInstanceName: instance.Name,
			SecretName:          bindingSecretName,
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, btpServiceBinding, func() error {
		return controllerutil.SetOwnerReference(cfServiceBinding, btpServiceBinding, r.scheme)
	})

	if err != nil {
		r.log.Error(err, "failed to create btp service binding")
		return ctrl.Result{}, err
	}

	err = r.k8sClient.Get(ctx, types.NamespacedName{Namespace: btpServiceBinding.Namespace, Name: btpServiceBinding.Spec.SecretName}, &corev1.Secret{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.log.Info("btp service binding secret not yet found", "secretName", btpServiceBinding.Spec.SecretName)
			return ctrl.Result{
				RequeueAfter: time.Second,
			}, nil
		}

		r.log.Error(err, "failed to get btp service binding secret")
		return ctrl.Result{}, err
	}

	cfServiceBinding.Status.Binding.Name = btpServiceBinding.Spec.SecretName
	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:    BindingSecretAvailableCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "SecretFound",
		Message: "",
	})

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		r.log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	if cfApp.Status.VCAPServicesSecretName == "" {
		r.log.Info("Did not find VCAPServiceSecret name on status of CFApp", "CFServiceBinding", cfServiceBinding.Name)
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:    VCAPServicesSecretAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "SecretNotFound",
			Message: "VCAPServicesSecret name absent from status of CFApp",
		})

		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:    VCAPServicesSecretAvailableCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "SecretFound",
		Message: "",
	})

	return ctrl.Result{}, nil
}

func (r *ManagedCFServiceBindingReconciler) handleGetError(ctx context.Context, err error, cfServiceBinding *korifiv1alpha1.CFServiceBinding, conditionType, notFoundReason, objectType string) (ctrl.Result, error) {
	cfServiceBinding.Status.Binding = corev1.LocalObjectReference{}
	if apierrors.IsNotFound(err) {
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionFalse,
			Reason:  notFoundReason,
			Message: objectType + " does not exist",
		})
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionFalse,
		Reason:  "UnknownError",
		Message: "Error occurred while fetching " + strings.ToLower(objectType) + ": " + err.Error(),
	})
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedCFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.isManaged))
}

func (r *ManagedCFServiceBindingReconciler) isManaged(object client.Object) bool {
	serviceBinding, ok := object.(*korifiv1alpha1.CFServiceBinding)
	if !ok {
		return true
	}

	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serviceBinding.Namespace,
			Name:      serviceBinding.Spec.Service.Name,
		},
	}

	err := r.k8sClient.Get(context.Background(), client.ObjectKeyFromObject(serviceInstance), serviceInstance)
	if err != nil {
		return false
	}

	return serviceInstance.Spec.Type == "managed"
}
