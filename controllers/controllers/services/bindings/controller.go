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

package bindings

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	BindingSecretAvailableCondition      = "BindingSecretAvailable"
	VCAPServicesSecretAvailableCondition = "VCAPServicesSecretAvailable"
	ServiceBindingGUIDLabel              = "korifi.cloudfoundry.org/service-binding-guid"
	ServiceCredentialBindingTypeLabel    = "korifi.cloudfoundry.org/service-credential-binding-type"
	ServiceBindingSecretTypePrefix       = "servicebinding.io/"
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

func (r *CFServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		Watches(
			&korifiv1alpha1.CFServiceInstance{},
			handler.EnqueueRequestsFromMapFunc(r.serviceInstanceToServiceBindings),
		)
}

func (r *CFServiceBindingReconciler) serviceInstanceToServiceBindings(ctx context.Context, o client.Object) []reconcile.Request {
	serviceInstance := o.(*korifiv1alpha1.CFServiceInstance)

	serviceBindings := korifiv1alpha1.CFServiceBindingList{}
	if err := r.k8sClient.List(ctx, &serviceBindings,
		client.InNamespace(serviceInstance.Namespace),
		client.MatchingFields{shared.IndexServiceBindingServiceInstanceGUID: serviceInstance.Name},
	); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, sb := range serviceBindings.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sb.Name,
				Namespace: sb.Namespace,
			},
		})
	}

	return requests
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=servicebinding.io,resources=servicebindings,verbs=get;list;create;update;patch;watch

func (r *CFServiceBindingReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfServiceBinding.Status.ObservedGeneration = cfServiceBinding.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceBinding.Status.ObservedGeneration)

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = controllerutil.SetOwnerReference(cfServiceInstance, cfServiceBinding, r.scheme)
	if err != nil {
		log.Info("error when making the service instance owner of the service binding", "reason", err)
		return ctrl.Result{}, err
	}

	if cfServiceInstance.Status.Credentials.Name == "" {
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               BindingSecretAvailableCondition,
			Status:             metav1.ConditionFalse,
			Reason:             "CredentialsSecretNotAvailable",
			Message:            "Service instance credentials not available yet",
			ObservedGeneration: cfServiceBinding.Generation,
		})
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	credentialsSecret, err := r.reconcileCredentials(ctx, cfServiceInstance, cfServiceBinding)
	if err != nil {
		log.Error(err, "failed to reconcile credentials secret")
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               BindingSecretAvailableCondition,
			Status:             metav1.ConditionFalse,
			Reason:             "FailedReconcilingCredentialsSecret",
			Message:            err.Error(),
			ObservedGeneration: cfServiceBinding.Generation,
		})
		return ctrl.Result{}, err
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	if cfApp.Status.VCAPServicesSecretName == "" {
		log.V(1).Info("did not find VCAPServiceSecret name on status of CFApp", "CFServiceBinding", cfServiceBinding.Name)
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
			Type:               VCAPServicesSecretAvailableCondition,
			Status:             metav1.ConditionFalse,
			Reason:             "SecretNotFound",
			Message:            "VCAPServicesSecret name absent from status of CFApp",
			ObservedGeneration: cfServiceBinding.Generation,
		})

		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:               VCAPServicesSecretAvailableCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "SecretFound",
		Message:            "",
		ObservedGeneration: cfServiceBinding.Generation,
	})

	actualSBServiceBinding := servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
		},
	}

	desiredSBServiceBinding := generateDesiredServiceBinding(&actualSBServiceBinding, cfServiceBinding, cfApp, credentialsSecret)

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, &actualSBServiceBinding, sbServiceBindingMutateFn(&actualSBServiceBinding, desiredSBServiceBinding))
	if err != nil {
		log.Info("error calling Create on servicebinding.io ServiceBinding", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFServiceBindingReconciler) reconcileCredentials(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*corev1.Secret, error) {
	cfServiceBinding.Status.Credentials.Name = cfServiceInstance.Status.Credentials.Name

	if isLegacyServiceBinding(cfServiceBinding, cfServiceInstance) {
		bindingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfServiceBinding.Status.Binding.Name,
				Namespace: cfServiceBinding.Namespace,
			},
		}

		// For legacy sevice bindings we want to keep the binding secret
		// unchanged in order to avoid unexpected app restarts. See ADR 16 for more details.
		err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)
		if err != nil {
			return nil, err
		}

		return bindingSecret, nil
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Status.Credentials.Name,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get service instance credentials secret %q: %w", cfServiceInstance.Status.Credentials.Name, err)
	}

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Type, err = credentials.GetBindingSecretType(credentialsSecret)
		if err != nil {
			return err
		}
		bindingSecret.Data, err = credentials.GetServiceBindingIOSecretData(credentialsSecret)
		if err != nil {
			return err
		}

		return controllerutil.SetControllerReference(cfServiceBinding, bindingSecret, r.scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create binding secret: %w", err)
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name
	meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
		Type:               BindingSecretAvailableCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "SecretAvailable",
		Message:            "",
		ObservedGeneration: cfServiceBinding.Generation,
	})

	return bindingSecret, nil
}

func isLegacyServiceBinding(cfServiceBinding *korifiv1alpha1.CFServiceBinding, cfServiceInstance *korifiv1alpha1.CFServiceInstance) bool {
	if cfServiceBinding.Status.Binding.Name == "" {
		return false
	}

	// When reconciling existing legacy service bindings we make
	// use of the fact that the service binding used to reference
	// the secret of the sevice instance that shares the sevice
	// instance name. See ADR 16 for more datails.
	return cfServiceInstance.Name == cfServiceBinding.Status.Binding.Name && cfServiceInstance.Spec.SecretName == cfServiceBinding.Status.Binding.Name
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

	bindingName := secret.Name
	if cfServiceBinding.Spec.DisplayName != nil {
		bindingName = *cfServiceBinding.Spec.DisplayName
	}

	desiredServiceBinding.Spec = servicebindingv1beta1.ServiceBindingSpec{
		Name: bindingName,
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
