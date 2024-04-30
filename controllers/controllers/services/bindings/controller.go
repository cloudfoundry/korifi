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
	"github.com/pkg/errors"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	ServiceBindingGUIDLabel           = "korifi.cloudfoundry.org/service-binding-guid"
	ServiceCredentialBindingTypeLabel = "korifi.cloudfoundry.org/service-credential-binding-type"
	ServiceBindingSecretTypePrefix    = "servicebinding.io/"
)

type Reconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	log       logr.Logger
}

func NewReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding] {
	cfBindingReconciler := &Reconciler{k8sClient: k8sClient, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding](log, k8sClient, cfBindingReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		Owns(&servicebindingv1beta1.ServiceBinding{}).
		Watches(
			&korifiv1alpha1.CFServiceInstance{},
			handler.EnqueueRequestsFromMapFunc(r.serviceInstanceToServiceBindings),
		).
		Watches(
			&korifiv1alpha1.CFApp{},
			handler.EnqueueRequestsFromMapFunc(r.appToServiceBindings),
		)
}

func (r *Reconciler) serviceInstanceToServiceBindings(ctx context.Context, o client.Object) []reconcile.Request {
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

func (r *Reconciler) appToServiceBindings(ctx context.Context, o client.Object) []reconcile.Request {
	cfApp := o.(*korifiv1alpha1.CFApp)

	serviceBindings := &korifiv1alpha1.CFServiceBindingList{}

	if err := r.k8sClient.List(ctx, serviceBindings,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name},
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

func (r *Reconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfServiceBinding.Status.ObservedGeneration = cfServiceBinding.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceBinding.Status.ObservedGeneration)

	var err error
	readyConditionBuilder := k8s.NewReadyConditionBuilder(cfServiceBinding)
	defer func() {
		meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, readyConditionBuilder.WithError(err).Build())
	}()

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetOwnerReference(cfServiceInstance, cfServiceBinding, r.scheme)
	if err != nil {
		log.Info("error when making the service instance owner of the service binding", "reason", err)
		return ctrl.Result{}, err
	}

	if cfServiceInstance.Status.Credentials.Name == "" {
		readyConditionBuilder.
			WithReason("CredentialsSecretNotAvailable").
			WithMessage("Service instance credentials not available yet")
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	credentialsSecret, err := r.reconcileCredentials(ctx, cfServiceInstance, cfServiceBinding)
	if err != nil {
		if k8serrors.IsInvalid(err) {
			err = r.k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfServiceBinding.Name,
					Namespace: cfServiceBinding.Namespace,
				},
			})
			return ctrl.Result{Requeue: true}, errors.Wrap(err, "failed to delete outdated binding secret")
		}

		log.Error(err, "failed to reconcile credentials secret")
		return ctrl.Result{}, err
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	sbServiceBinding, err := r.reconcileSBServiceBinding(ctx, cfServiceBinding, credentialsSecret)
	if err != nil {
		log.Info("error creating/updating servicebinding.io servicebinding", "reason", err)
		return ctrl.Result{}, err
	}

	if !isSbServiceBindingReady(sbServiceBinding) {
		readyConditionBuilder.WithReason("ServiceBindingNotReady")
		return ctrl.Result{}, nil
	}

	readyConditionBuilder.Ready()
	return ctrl.Result{}, nil
}

func isSbServiceBindingReady(sbServiceBinding *servicebindingv1beta1.ServiceBinding) bool {
	readyCondition := meta.FindStatusCondition(sbServiceBinding.Status.Conditions, "Ready")
	if readyCondition == nil {
		return false
	}

	if readyCondition.Status != metav1.ConditionTrue {
		return false
	}

	return sbServiceBinding.Generation == sbServiceBinding.Status.ObservedGeneration
}

func (r *Reconciler) reconcileCredentials(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*corev1.Secret, error) {
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
		return nil, errors.Wrap(err, "failed to create binding secret")
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name

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

func (r *Reconciler) reconcileSBServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, credentialsSecret *corev1.Secret) (*servicebindingv1beta1.ServiceBinding, error) {
	sbServiceBinding := r.toSBServiceBinding(cfServiceBinding)

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, sbServiceBinding, func() error {
		sbServiceBinding.Spec.Name = getSBServiceBindingName(cfServiceBinding)

		secretType, hasType := credentialsSecret.Data["type"]
		if hasType && len(secretType) > 0 {
			sbServiceBinding.Spec.Type = string(secretType)
		}

		secretProvider, hasProvider := credentialsSecret.Data["provider"]
		if hasProvider {
			sbServiceBinding.Spec.Provider = string(secretProvider)
		}
		return controllerutil.SetControllerReference(cfServiceBinding, sbServiceBinding, r.scheme)
	})
	if err != nil {
		return nil, err
	}

	return sbServiceBinding, nil
}

func (r *Reconciler) toSBServiceBinding(cfServiceBinding *korifiv1alpha1.CFServiceBinding) *servicebindingv1beta1.ServiceBinding {
	return &servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
			Labels: map[string]string{
				ServiceBindingGUIDLabel:           cfServiceBinding.Name,
				korifiv1alpha1.CFAppGUIDLabelKey:  cfServiceBinding.Spec.AppRef.Name,
				ServiceCredentialBindingTypeLabel: "app",
			},
		},
		Spec: servicebindingv1beta1.ServiceBindingSpec{
			Type: "user-provided",
			Workload: servicebindingv1beta1.ServiceBindingWorkloadReference{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						korifiv1alpha1.CFAppGUIDLabelKey: cfServiceBinding.Spec.AppRef.Name,
					},
				},
			},
			Service: servicebindingv1beta1.ServiceBindingServiceReference{
				APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				Kind:       "CFServiceBinding",
				Name:       cfServiceBinding.Name,
			},
		},
	}
}

func getSBServiceBindingName(cfServiceBinding *korifiv1alpha1.CFServiceBinding) string {
	if cfServiceBinding.Spec.DisplayName != nil {
		return *cfServiceBinding.Spec.DisplayName
	}

	return cfServiceBinding.Status.Binding.Name
}
