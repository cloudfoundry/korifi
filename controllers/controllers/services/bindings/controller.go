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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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
	ServiceBindingGUIDLabel           = "korifi.cloudfoundry.org/service-binding-guid"
	ServiceCredentialBindingTypeLabel = "korifi.cloudfoundry.org/service-credential-binding-type"
	ServiceBindingSecretTypePrefix    = "servicebinding.io/"
)

type CredentialsReconciler interface {
	ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error)
}

type Reconciler struct {
	k8sClient                 client.Client
	scheme                    *runtime.Scheme
	log                       logr.Logger
	upsiCredentialsReconciler CredentialsReconciler
}

func NewReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	upsiCredentialsReconciler CredentialsReconciler,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding] {
	cfBindingReconciler := &Reconciler{k8sClient: k8sClient, scheme: scheme, log: log, upsiCredentialsReconciler: upsiCredentialsReconciler}
	return k8s.NewPatchingReconciler(log, k8sClient, cfBindingReconciler)
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

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	cfServiceBinding.Status.ObservedGeneration = cfServiceBinding.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceBinding.Status.ObservedGeneration)

	err = controllerutil.SetOwnerReference(cfServiceInstance, cfServiceBinding, r.scheme)
	if err != nil {
		log.Info("error when making the service instance owner of the service binding", "reason", err)
		return ctrl.Result{}, err
	}

	res, err := r.upsiCredentialsReconciler.ReconcileResource(ctx, cfServiceBinding)
	if needsRequeue(res) || err != nil {
		return res, err
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	sbServiceBinding, err := r.reconcileSBServiceBinding(ctx, cfServiceBinding)
	if err != nil {
		log.Info("error creating/updating servicebinding.io servicebinding", "reason", err)
		return ctrl.Result{}, err
	}

	if !isSbServiceBindingReady(sbServiceBinding) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ServiceBindingNotReady")
	}

	return ctrl.Result{}, nil
}

func needsRequeue(res ctrl.Result) bool {
	return !res.IsZero()
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

func (r *Reconciler) reconcileSBServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*servicebindingv1beta1.ServiceBinding, error) {
	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}

	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get service binding credentials secret %q: %w", cfServiceBinding.Status.Binding.Name, err)
	}

	sbServiceBinding := r.toSBServiceBinding(cfServiceBinding)

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, sbServiceBinding, func() error {
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
