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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type DelegateReconciler interface {
	ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error)
}

type Reconciler struct {
	k8sClient         client.Client
	scheme            *runtime.Scheme
	log               logr.Logger
	upsiReconciler    DelegateReconciler
	managedReconciler DelegateReconciler
}

func NewReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	upsiCredentialsReconciler DelegateReconciler,
	managedCredentialsReconciler DelegateReconciler,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding] {
	cfBindingReconciler := &Reconciler{
		k8sClient:         k8sClient,
		scheme:            scheme,
		log:               log,
		upsiReconciler:    upsiCredentialsReconciler,
		managedReconciler: managedCredentialsReconciler,
	}
	return k8s.NewPatchingReconciler(log, k8sClient, cfBindingReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		Watches(
			&korifiv1alpha1.CFServiceInstance{},
			handler.EnqueueRequestsFromMapFunc(r.serviceInstanceToServiceBindings),
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

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings/finalizers,verbs=update

func (r *Reconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfServiceBinding.Status.ObservedGeneration = cfServiceBinding.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceBinding.Status.ObservedGeneration)

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	cfServiceBinding.Annotations = tools.SetMapValue(cfServiceBinding.Annotations, korifiv1alpha1.ServiceInstanceTypeAnnotationKey, string(cfServiceInstance.Spec.Type))

	if err = k8s.Patch(ctx, r.k8sClient, cfServiceInstance, func() {
		controllerutil.AddFinalizer(cfServiceInstance, metav1.FinalizerDeleteDependents)
	}); err != nil {
		log.Info("error when setting the foreground deletion finalizer on the service instance", "reason", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetOwnerReference(cfServiceInstance, cfServiceBinding, r.scheme, controllerutil.WithBlockOwnerDeletion(true))
	if err != nil {
		log.Info("error when making the service instance owner of the service binding", "reason", err)
		return ctrl.Result{}, err
	}

	res, err := r.reconcileByType(ctx, cfServiceInstance, cfServiceBinding)
	if needsRequeue(res, err) {
		if err != nil {
			log.Error(err, "failed to reconcile binding credentials")
		}
		return res, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileByType(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	if cfServiceInstance.Spec.Type == korifiv1alpha1.UserProvidedType {
		return r.upsiReconciler.ReconcileResource(ctx, cfServiceBinding)
	}

	return r.managedReconciler.ReconcileResource(ctx, cfServiceBinding)
}

func needsRequeue(res ctrl.Result, err error) bool {
	if err != nil {
		return true
	}

	return !res.IsZero()
}
