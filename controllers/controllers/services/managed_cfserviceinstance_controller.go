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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	btpv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagedCFServiceInstanceReconciler reconciles a CFServiceInstance object
type ManagedCFServiceInstanceReconciler struct {
	k8sClient     client.Client
	scheme        *runtime.Scheme
	log           logr.Logger
	rootNamespace string
}

func NewManagedCFServiceInstanceReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	rootNamespace string,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance] {
	serviceInstanceReconciler := ManagedCFServiceInstanceReconciler{
		k8sClient:     client,
		scheme:        scheme,
		log:           log,
		rootNamespace: rootNamespace,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance](log, client, &serviceInstanceReconciler)
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceofferings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceplans,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=services.cloud.sap.com,resources=servicebindings,verbs=get;list;create;update;patch;watch;delete

func (r *ManagedCFServiceInstanceReconciler) ReconcileResource(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	if !cfServiceInstance.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFServiceInstance(ctx, cfServiceInstance)
	}

	servicePlan, err := r.getServicePlan(ctx, cfServiceInstance.Spec.ServicePlanGUID)
	if err != nil {
		return ctrl.Result{}, err
	}

	serviceOffering, err := r.getServiceOffering(ctx, servicePlan.Spec.Relationships.ServiceOfferingGUID)
	if err != nil {
		return ctrl.Result{}, err
	}

	btpServiceInstance := &btpv1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Name,
		},
		Spec: btpv1.ServiceInstanceSpec{
			ServiceOfferingName: serviceOffering.Spec.OfferingName,
			ServicePlanName:     servicePlan.Spec.PlanName,
			Parameters:          cfServiceInstance.Spec.Parameters,
		},
	}

	err = r.k8sClient.Create(ctx, btpServiceInstance)
	if client.IgnoreAlreadyExists(err) != nil {
		return ctrl.Result{}, err
	}
	err = r.k8sClient.Get(ctx, client.ObjectKeyFromObject(btpServiceInstance), btpServiceInstance)
	if err != nil {
		return ctrl.Result{}, err
	}

	btpServiceBinding := &btpv1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Name,
		},
		Spec: btpv1.ServiceBindingSpec{
			ServiceInstanceName: cfServiceInstance.Name,
			SecretName:          cfServiceInstance.Spec.SecretName,
		},
	}

	err = r.k8sClient.Create(ctx, btpServiceBinding)
	if client.IgnoreAlreadyExists(err) != nil {
		r.log.Error(err, "failed to create btp service binding")
		return ctrl.Result{}, err
	}

	err = r.k8sClient.Get(ctx, client.ObjectKeyFromObject(btpServiceBinding), btpServiceBinding)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.k8sClient.Get(ctx, types.NamespacedName{Namespace: btpServiceBinding.Namespace, Name: btpServiceBinding.Spec.SecretName}, &corev1.Secret{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.log.Info("btp service binding secret not yet found", "secretName", btpServiceBinding.Spec.SecretName)
			cfServiceInstance.Status = bindSecretUnavailableStatus(cfServiceInstance, "BTPSecretNotAvailable", "BTP service secret not available yet")
			return ctrl.Result{
				RequeueAfter: time.Second,
			}, nil
		}

		r.log.Error(err, "failed to get btp service binding secret")
		cfServiceInstance.Status = bindSecretUnavailableStatus(cfServiceInstance, "UnknownError", "BTP service secret could not be retrieved: "+err.Error())
		return ctrl.Result{}, err
	}

	cfServiceInstance.Status = bindSecretAvailableStatus(cfServiceInstance)
	return ctrl.Result{}, nil
}

func (r *ManagedCFServiceInstanceReconciler) finalizeCFServiceInstance(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFServiceInstance")

	if !controllerutil.ContainsFinalizer(cfServiceInstance, korifiv1alpha1.ManagedCFServiceInstanceFinalizerName) {
		return ctrl.Result{}, nil
	}

	bindingDeleteErr := r.k8sClient.Delete(ctx, &btpv1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Name,
		},
	})
	if bindingDeleteErr != nil {
		log.V(1).Info("deleting BTP service binding failed", "error", bindingDeleteErr)
	}

	instanceDeleteErr := r.k8sClient.Delete(ctx, &btpv1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Name,
		},
	})
	if instanceDeleteErr != nil {
		log.V(1).Info("deleting BTP service instance failed", "error", instanceDeleteErr)
	}

	if k8serrors.IsNotFound(bindingDeleteErr) && k8serrors.IsNotFound(instanceDeleteErr) {
		if controllerutil.RemoveFinalizer(cfServiceInstance, korifiv1alpha1.ManagedCFServiceInstanceFinalizerName) {
			log.V(1).Info("finalizer removed")
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{RequeueAfter: time.Second}, nil
}

func (r *ManagedCFServiceInstanceReconciler) getServicePlan(ctx context.Context, servicePlanGuid string) (korifiv1alpha1.CFServicePlan, error) {
	servicePlans := korifiv1alpha1.CFServicePlanList{}
	err := r.k8sClient.List(ctx, &servicePlans, client.InNamespace(r.rootNamespace), client.MatchingFields{shared.IndexServicePlanGUID: servicePlanGuid})
	if err != nil {
		return korifiv1alpha1.CFServicePlan{}, err
	}

	if len(servicePlans.Items) != 1 {
		return korifiv1alpha1.CFServicePlan{}, fmt.Errorf("found %d service plans for guid %q, expected one", len(servicePlans.Items), servicePlanGuid)
	}

	return servicePlans.Items[0], nil
}

func (r *ManagedCFServiceInstanceReconciler) getServiceOffering(ctx context.Context, serviceOfferingGuid string) (korifiv1alpha1.CFServiceOffering, error) {
	serviceOfferings := korifiv1alpha1.CFServiceOfferingList{}
	err := r.k8sClient.List(ctx, &serviceOfferings, client.InNamespace(r.rootNamespace), client.MatchingFields{shared.IndexServiceOfferingID: serviceOfferingGuid})
	if err != nil {
		return korifiv1alpha1.CFServiceOffering{}, err
	}

	if len(serviceOfferings.Items) != 1 {
		return korifiv1alpha1.CFServiceOffering{}, fmt.Errorf("found %d service offerings for guid %q, expected one", len(serviceOfferings.Items), serviceOfferingGuid)
	}

	return serviceOfferings.Items[0], nil
}

func (r *ManagedCFServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.isManaged)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.secretToCfServiceInstance))
}

func (r *ManagedCFServiceInstanceReconciler) isManaged(object client.Object) bool {
	serviceInstance, ok := object.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return true
	}

	return serviceInstance.Spec.Type == "managed"
}

func (r *ManagedCFServiceInstanceReconciler) secretToCfServiceInstance(ctx context.Context, object client.Object) []reconcile.Request {
	secret, ok := object.(*corev1.Secret)
	if !ok {
		r.log.Error(fmt.Errorf("unexpected object %T, expected corev1.Secret", object), "object", object)
		return []reconcile.Request{}
	}

	cfServiceInstanceList := &korifiv1alpha1.CFServiceInstanceList{}
	err := r.k8sClient.List(ctx, cfServiceInstanceList, client.InNamespace(secret.Namespace))
	if err != nil {
		r.log.Error(err, "failed to list service bindings in namespace", "namespace", secret.Namespace)
		return []reconcile.Request{}
	}

	result := []reconcile.Request{}
	for _, cfServiceInstance := range cfServiceInstanceList.Items {
		if cfServiceInstance.Spec.SecretName == secret.Name {
			result = append(result, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&cfServiceInstance)})
		}
	}

	return result
}
