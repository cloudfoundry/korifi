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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	requeueInterval               = 5 * time.Second
	ReadyCondition                = "Ready"
	FailedCondition               = "Failed"
	ProvisionRequestedCondition   = "PriovisionRequested"
	DeprovisionRequestedCondition = "DepriovisionRequested"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/status,verbs=get;update;patch

// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceofferings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceplans,verbs=get;list;watch;create;update;patch;delete
type ManagedCFServiceInstanceReconciler struct {
	log           logr.Logger
	k8sClient     client.Client
	brokerClient  BrokerClient
	rootNamespace string
}

func NewManagedCFServiceInstanceReconciler(
	log logr.Logger,
	k8sClient client.Client,
	brokerClient BrokerClient,
	rootNamespace string,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance] {
	serviceInstanceReconciler := ManagedCFServiceInstanceReconciler{
		log:           log,
		k8sClient:     k8sClient,
		brokerClient:  brokerClient,
		rootNamespace: rootNamespace,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance](log, k8sClient, &serviceInstanceReconciler)
}

func (r *ManagedCFServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.isManaged))
}

func (r *ManagedCFServiceInstanceReconciler) isManaged(object client.Object) bool {
	serviceInstance, ok := object.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		r.log.Error(fmt.Errorf("expected CFServiceInstance, got %T", object), "unexpected object")
		return false
	}

	return serviceInstance.Spec.Type == korifiv1alpha1.ManagedType
}

func (r *ManagedCFServiceInstanceReconciler) ReconcileResource(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	if !cfServiceInstance.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFServiceInstance(ctx, cfServiceInstance)
	}

	log := logr.FromContextOrDiscard(ctx).WithName("managed-cf-service-instance-controller")
	cfServiceInstance.Status.ObservedGeneration = cfServiceInstance.Generation
	log.Info("set observed generation", "generation", cfServiceInstance.Status.ObservedGeneration)

	if meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, ReadyCondition) ||
		meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, FailedCondition) {
		// service instance already provisioned, nothing more to do
		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, ProvisionRequestedCondition) {
		planVisible, err := r.servicePlanVisible(ctx, cfServiceInstance)
		if err != nil {
			log.Error(err, "failed cheching for service plan visibility")
			return ctrl.Result{}, err
		}

		if !planVisible {
			meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
				Type:               FailedCondition,
				Status:             metav1.ConditionTrue,
				Reason:             "PlanNotAvailable",
				Message:            "Service plan is not visible in current organization",
				ObservedGeneration: cfServiceInstance.Generation,
			})
			return ctrl.Result{}, nil
		}

		err = r.brokerClient.ProvisionServiceInstance(ctx, cfServiceInstance)
		if err != nil {
			log.Error(err, "provisioning request failed")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
			Type:               ProvisionRequestedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "ProvisionRequested",
			Message:            "Service provisioning has been requested from broker",
			ObservedGeneration: cfServiceInstance.Generation,
		})

		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	lastOp, err := r.brokerClient.GetServiceInstanceLastOperation(ctx, cfServiceInstance)
	if err != nil {
		log.Error(err, "get state failed")
		return ctrl.Result{}, err
	}
	if !lastOp.Exists {
		return ctrl.Result{}, fmt.Errorf("last operation for service instance %q not found", cfServiceInstance.Name)
	}

	if lastOp.State == "succeeded" {
		meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
			Type:               ReadyCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			Message:            lastOp.Description,
			ObservedGeneration: cfServiceInstance.Generation,
		})

		return ctrl.Result{}, nil
	}

	if lastOp.State == "failed" {
		meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
			Type:               FailedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "Failed",
			Message:            lastOp.Description,
			ObservedGeneration: cfServiceInstance.Generation,
		})

		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ManagedCFServiceInstanceReconciler) servicePlanVisible(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (bool, error) {
	plan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      cfServiceInstance.Spec.ServicePlanGUID,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(plan), plan)
	if err != nil {
		return false, fmt.Errorf("failed to get service plan %q: %w", cfServiceInstance.Spec.ServicePlanGUID, err)
	}

	spaces := &korifiv1alpha1.CFSpaceList{}
	err = r.k8sClient.List(ctx, spaces, client.MatchingFields{shared.IndexSpaceNamespaceName: cfServiceInstance.Namespace})
	if err != nil {
		return false, fmt.Errorf("failed to list CFSpaces: %w", err)
	}
	if len(spaces.Items) != 1 {
		return false, fmt.Errorf("one CFSpace with guid %q expected, %d found", cfServiceInstance.Namespace, len(spaces.Items))
	}

	return plan.IsVisible(spaces.Items[0].Namespace), nil
}

func (r *ManagedCFServiceInstanceReconciler) finalizeCFServiceInstance(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFServiceInstance")

	if !controllerutil.ContainsFinalizer(cfServiceInstance, korifiv1alpha1.ManagedCFServiceInstanceFinalizerName) {
		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, DeprovisionRequestedCondition) {
		bindingsDeleted, err := r.deleteBindings(ctx, cfServiceInstance)
		if err != nil {
			log.Error(err, "failed to delete service bindings")
			return ctrl.Result{}, err

		}
		if !bindingsDeleted {
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}

		err = r.brokerClient.DeprovisionServiceInstance(ctx, cfServiceInstance)
		if err != nil {
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
			Type:               DeprovisionRequestedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             "DeprovisionRequested",
			ObservedGeneration: cfServiceInstance.Generation,
		})

		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	lastOp, err := r.brokerClient.GetServiceInstanceLastOperation(ctx, cfServiceInstance)
	if !lastOp.Exists {
		// the service instance is gone
		if controllerutil.RemoveFinalizer(cfServiceInstance, korifiv1alpha1.ManagedCFServiceInstanceFinalizerName) {
			log.V(1).Info("finalizer removed")
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ManagedCFServiceInstanceReconciler) deleteBindings(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (bool, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("deleteBindings")

	serviceBindings := &korifiv1alpha1.CFServiceBindingList{}
	err := r.k8sClient.List(
		ctx,
		serviceBindings,
		client.InNamespace(cfServiceInstance.Namespace),
		client.MatchingFields{shared.IndexServiceBindingServiceInstanceGUID: cfServiceInstance.Name},
	)
	if err != nil {
		log.Error(err, "failed to list service bindings for service instance", "instanceName", cfServiceInstance.Name)
		return false, err
	}

	if len(serviceBindings.Items) != 0 {
		for i := range serviceBindings.Items {
			err = r.k8sClient.Delete(ctx, &serviceBindings.Items[i])
			if err != nil {
				log.Error(err, "failed to delete service binding", "bindingName", serviceBindings.Items[i].Name)
				return false, err
			}
		}
		return false, nil
	}

	return true, nil
}
