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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CFServiceInstanceReconciler reconciles a CFServiceInstance object
type CFServiceInstanceReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewCFServiceInstanceReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance] {
	serviceInstanceReconciler := CFServiceInstanceReconciler{k8sClient: client, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance](log, client, &serviceInstanceReconciler)
}

func (r *CFServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{})
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/status,verbs=get;update;patch

func (r *CFServiceInstanceReconciler) ReconcileResource(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	cfServiceInstance.Status.ObservedGeneration = cfServiceInstance.Generation
	r.log.V(1).Info("set observed generation", "generation", cfServiceInstance.Status.ObservedGeneration)

	secret := new(corev1.Secret)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceInstance.Spec.SecretName, Namespace: cfServiceInstance.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cfServiceInstance.Status = bindSecretUnavailableStatus(cfServiceInstance, "SecretNotFound", "Binding secret does not exist")
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}

		cfServiceInstance.Status = bindSecretUnavailableStatus(cfServiceInstance, "UnknownError", "Error occurred while fetching secret: "+err.Error())
		return ctrl.Result{}, err
	}

	cfServiceInstance.Status = bindSecretAvailableStatus(cfServiceInstance)
	return ctrl.Result{}, nil
}

func bindSecretAvailableStatus(cfServiceInstance *korifiv1alpha1.CFServiceInstance) korifiv1alpha1.CFServiceInstanceStatus {
	status := korifiv1alpha1.CFServiceInstanceStatus{
		Binding: corev1.LocalObjectReference{
			Name: cfServiceInstance.Spec.SecretName,
		},
		Conditions:         cfServiceInstance.Status.Conditions,
		ObservedGeneration: cfServiceInstance.Status.ObservedGeneration,
	}

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               BindingSecretAvailableCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "SecretFound",
		ObservedGeneration: cfServiceInstance.Generation,
	})

	return status
}

func bindSecretUnavailableStatus(cfServiceInstance *korifiv1alpha1.CFServiceInstance, reason, message string) korifiv1alpha1.CFServiceInstanceStatus {
	status := korifiv1alpha1.CFServiceInstanceStatus{
		Binding:            corev1.LocalObjectReference{},
		Conditions:         cfServiceInstance.Status.Conditions,
		ObservedGeneration: cfServiceInstance.Status.ObservedGeneration,
	}

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               BindingSecretAvailableCondition,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cfServiceInstance.Generation,
	})

	return status
}
