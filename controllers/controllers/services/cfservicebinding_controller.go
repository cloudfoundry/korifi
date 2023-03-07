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
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// CFServiceBindingReconciler reconciles a CFServiceBinding object
type CFServiceBindingReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewCFServiceBindingReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding] {
	cfBindingReconciler := &CFServiceBindingReconciler{k8sClient: k8sClient, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding](log, k8sClient, cfBindingReconciler)
}

const (
	BindingSecretAvailableCondition      = "BindingSecretAvailable"
	VCAPServicesSecretAvailableCondition = "VCAPServicesSecretAvailable"
	ServiceBindingGUIDLabel              = "korifi.cloudfoundry.org/service-binding-guid"
	ServiceCredentialBindingTypeLabel    = "korifi.cloudfoundry.org/service-credential-binding-type"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=servicebinding.io,resources=servicebindings,verbs=get;list;create;update;patch;watch

func (r *CFServiceBindingReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	instance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, instance)
	if err != nil {
		// Unlike with CFApp cascading delete, CFServiceInstance delete cleans up CFServiceBindings itself as part of finalizing,
		// so we do not check for deletion timestamp before returning here.
		return r.handleGetError(ctx, err, cfServiceBinding, BindingSecretAvailableCondition, "ServiceInstanceNotFound", "Service instance")
	}

	err = controllerutil.SetOwnerReference(instance, cfServiceBinding, r.scheme)
	if err != nil {
		r.log.Info("error when making the service instance owner of the service binding", "reason", err)
		return ctrl.Result{}, err
	}

	secret := new(corev1.Secret)
	// Note: is there a reason to fetch the secret name from the service instance spec?
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: instance.Spec.SecretName, Namespace: cfServiceBinding.Namespace}, secret)
	if err != nil {
		return r.handleGetError(ctx, err, cfServiceBinding, BindingSecretAvailableCondition, "SecretNotFound", "Binding secret")
	}

	cfServiceBinding.Status.Binding.Name = instance.Spec.SecretName
	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:    BindingSecretAvailableCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "SecretFound",
		Message: "",
	})

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		r.log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	if cfApp.Status.VCAPServicesSecretName == "" {
		r.log.V(1).Info("did not find VCAPServiceSecret name on status of CFApp", "CFServiceBinding", cfServiceBinding.Name)
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

	actualSBServiceBinding := servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
		},
	}

	desiredSBServiceBinding := generateDesiredServiceBinding(&actualSBServiceBinding, cfServiceBinding, cfApp, secret)

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, &actualSBServiceBinding, sbServiceBindingMutateFn(&actualSBServiceBinding, desiredSBServiceBinding))
	if err != nil {
		r.log.Info("error calling Create on servicebinding.io ServiceBinding", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFServiceBindingReconciler) handleGetError(ctx context.Context, err error, cfServiceBinding *korifiv1alpha1.CFServiceBinding, conditionType, notFoundReason, objectType string) (ctrl.Result, error) {
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

func sbServiceBindingMutateFn(actualSBServiceBinding, desiredSBServiceBinding *servicebindingv1beta1.ServiceBinding) controllerutil.MutateFn {
	return func() error {
		actualSBServiceBinding.Labels = desiredSBServiceBinding.Labels
		actualSBServiceBinding.OwnerReferences = desiredSBServiceBinding.OwnerReferences
		actualSBServiceBinding.Spec = desiredSBServiceBinding.Spec
		return nil
	}
}

func generateDesiredServiceBinding(actualServiceBinding *servicebindingv1beta1.ServiceBinding, cfServiceBinding *korifiv1alpha1.CFServiceBinding, cfApp *korifiv1alpha1.CFApp, secret *corev1.Secret) *servicebindingv1beta1.ServiceBinding {
	var desiredServiceBinding servicebindingv1beta1.ServiceBinding
	actualServiceBinding.DeepCopyInto(&desiredServiceBinding)
	desiredServiceBinding.Labels = map[string]string{
		ServiceBindingGUIDLabel:           cfServiceBinding.Name,
		korifiv1alpha1.CFAppGUIDLabelKey:  cfApp.Name,
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
					korifiv1alpha1.CFAppGUIDLabelKey: cfApp.Name,
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

// SetupWithManager sets up the controller with the Manager.
func (r *CFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.isUPSI))
}

func (r *CFServiceBindingReconciler) isUPSI(object client.Object) bool {
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

	return serviceInstance.Spec.Type == "user-provided"
}
