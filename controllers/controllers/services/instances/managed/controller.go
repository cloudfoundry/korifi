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

package managed

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	k8sClient           client.Client
	osbapiClientFactory osbapi.BrokerClientFactory
	scheme              *runtime.Scheme
	rootNamespace       string
	log                 logr.Logger
	assets              *osbapi.Assets
}

func NewReconciler(
	client client.Client,
	brokerClientFactory osbapi.BrokerClientFactory,
	scheme *runtime.Scheme,
	rootNamespace string,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance] {
	return k8s.NewPatchingReconciler(log, client, &Reconciler{
		k8sClient:           client,
		osbapiClientFactory: brokerClientFactory,
		scheme:              scheme,
		rootNamespace:       rootNamespace,
		log:                 log,
		assets:              osbapi.NewAssets(client, rootNamespace),
	})
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		Named("managed-cfserviceinstance").
		WithEventFilter(predicate.NewPredicateFuncs(r.isManaged)).
		Watches(
			&korifiv1alpha1.CFServicePlan{},
			handler.EnqueueRequestsFromMapFunc(r.servicePlanToServiceInstances),
		)
}

func (r *Reconciler) servicePlanToServiceInstances(ctx context.Context, o client.Object) []reconcile.Request {
	servicePlan := o.(*korifiv1alpha1.CFServicePlan)

	serviceInstancesList := korifiv1alpha1.CFServiceInstanceList{}
	if err := r.k8sClient.List(ctx, &serviceInstancesList,
		client.MatchingFields{shared.IndexServiceInstancePlanGUID: servicePlan.Name},
	); err != nil {
		return []reconcile.Request{}
	}

	serviceInstances := it.Map(slices.Values(serviceInstancesList.Items),
		func(si korifiv1alpha1.CFServiceInstance) client.Object {
			return &si
		},
	)

	return slices.Collect(it.Map(it.Filter(serviceInstances, r.isManaged),
		func(si client.Object) reconcile.Request {
			return reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(si),
			}
		}))
}

func (r *Reconciler) isManaged(object client.Object) bool {
	serviceInstance, ok := object.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return true
	}

	return serviceInstance.Spec.Type == korifiv1alpha1.ManagedType
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/status,verbs=get;update;atch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceinstances/finalizers,verbs=update

func (r *Reconciler) ReconcileResource(ctx context.Context, serviceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	serviceInstance.Status.ObservedGeneration = serviceInstance.Generation
	log.V(1).Info("set observed generation", "generation", serviceInstance.Status.ObservedGeneration)

	if !serviceInstance.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFServiceInstance(ctx, serviceInstance)
	}

	if isReady(serviceInstance) {
		return ctrl.Result{}, nil
	}

	if isFailed(serviceInstance) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ProvisioningFailed").WithNoRequeue()
	}

	serviceInstanceAssets, err := r.assets.GetServiceInstanceAssets(ctx, serviceInstance)
	if err != nil {
		log.Error(err, "failed to get service instance assets")
		return ctrl.Result{}, err
	}

	planVisible, err := r.isServicePlanVisible(ctx, serviceInstance, serviceInstanceAssets.ServicePlan)
	if err != nil {
		log.Error(err, "failed to check service plan visibility")
		return ctrl.Result{}, err
	}

	if !planVisible {
		return ctrl.Result{},
			k8s.NewNotReadyError().WithMessage("The service plan is disabled").WithReason("InvalidServicePlan").WithNoRequeue()
	}

	if serviceInstance.Spec.ServiceLabel == nil {
		serviceInstance.Spec.ServiceLabel = tools.PtrTo(serviceInstanceAssets.ServiceOffering.Spec.Name)
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, serviceInstanceAssets.ServiceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", serviceInstanceAssets.ServiceBroker.Name)
		return ctrl.Result{}, fmt.Errorf("failed to create client for broker %q: %w", serviceInstanceAssets.ServiceBroker.Name, err)
	}

	if !isProvisionRequested(serviceInstance) {
		return r.provisionServiceInstance(ctx, serviceInstance, serviceInstanceAssets, osbapiClient)
	}

	return r.pollProvisionOperation(ctx, serviceInstance, serviceInstanceAssets, osbapiClient)
}

func (r *Reconciler) isServicePlanVisible(
	ctx context.Context,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
	servicePlan *korifiv1alpha1.CFServicePlan,
) (bool, error) {
	if servicePlan.Spec.Visibility.Type == korifiv1alpha1.AdminServicePlanVisibilityType {
		return false, nil
	}

	if servicePlan.Spec.Visibility.Type == korifiv1alpha1.PublicServicePlanVisibilityType {
		return true, nil
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceInstance.Namespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace)
	if err != nil {
		return false, err
	}

	return slices.Contains(servicePlan.Spec.Visibility.Organizations, namespace.Labels[korifiv1alpha1.OrgGUIDKey]), nil
}

func (r *Reconciler) provisionServiceInstance(
	ctx context.Context,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
	assets osbapi.ServiceInstanceAssets,
	osbapiClient osbapi.BrokerClient,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("provision-service-instance")

	parametersMap, err := getServiceInstanceParameters(serviceInstance)
	if err != nil {
		log.Error(err, "failed to get service instance parameters")
		return ctrl.Result{}, fmt.Errorf("failed to get service instance parameters: %w", err)
	}

	namespace, err := r.getNamespace(ctx, serviceInstance.Namespace)
	if err != nil {
		log.Error(err, "failed to get namespace")
		return ctrl.Result{}, err
	}

	var provisionResponse osbapi.ServiceInstanceOperationResponse
	provisionResponse, err = osbapiClient.Provision(ctx, osbapi.InstanceProvisionPayload{
		InstanceID: serviceInstance.Name,
		InstanceProvisionRequest: osbapi.InstanceProvisionRequest{
			ServiceId:  assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:     assets.ServicePlan.Spec.BrokerCatalog.ID,
			SpaceGUID:  namespace.Labels[korifiv1alpha1.SpaceGUIDKey],
			OrgGUID:    namespace.Labels[korifiv1alpha1.OrgGUIDKey],
			Parameters: parametersMap,
		},
	})
	if err != nil {
		log.Error(err, "failed to provision service")

		meta.SetStatusCondition(&serviceInstance.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.ProvisioningFailedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: serviceInstance.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "ProvisionFailed",
			Message:            err.Error(),
		})
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ProvisionFailed")
	}

	if provisionResponse.Complete {
		return ctrl.Result{}, nil
	}

	serviceInstance.Status.ProvisionOperation = provisionResponse.Operation
	meta.SetStatusCondition(&serviceInstance.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.ProvisionRequestedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: serviceInstance.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "ProvisionRequested",
	})

	return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ProvisionRequested").WithRequeue()
}

func (r *Reconciler) pollProvisionOperation(
	ctx context.Context,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
	assets osbapi.ServiceInstanceAssets,
	osbapiClient osbapi.BrokerClient,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("poll-provision-operation")

	lastOpResponse, err := osbapiClient.GetServiceInstanceLastOperation(ctx, osbapi.GetServiceInstanceLastOperationRequest{
		InstanceID: serviceInstance.Name,
		GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
			ServiceId: assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:    assets.ServicePlan.Spec.BrokerCatalog.ID,
			Operation: serviceInstance.Status.ProvisionOperation,
		},
	})
	if err != nil {
		log.Error(err, "getting service instance last operation failed")
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("GetLastOperationFailed")
	}

	if lastOpResponse.State == "in progress" {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ProvisionInProgress").WithRequeue()
	}

	if lastOpResponse.State == "failed" {
		meta.SetStatusCondition(&serviceInstance.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.ProvisioningFailedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: serviceInstance.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "ProvisionFailed",
			Message:            lastOpResponse.Description,
		})
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("ProvisionFailed")
	}

	return ctrl.Result{}, nil
}

func getServiceInstanceParameters(serviceInstance *korifiv1alpha1.CFServiceInstance) (map[string]any, error) {
	if serviceInstance.Spec.Parameters == nil {
		return nil, nil
	}

	parametersMap := map[string]any{}
	err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &parametersMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return parametersMap, nil
}

func (r *Reconciler) finalizeCFServiceInstance(
	ctx context.Context,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFServiceInstance")

	if !controllerutil.ContainsFinalizer(serviceInstance, korifiv1alpha1.CFManagedServiceInstanceFinalizerName) {
		return ctrl.Result{}, nil
	}

	if !isDeprovisionRequested(serviceInstance) {
		r.deprovisionServiceInstance(ctx, serviceInstance)

		meta.SetStatusCondition(&serviceInstance.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.DeprovisionRequestedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: serviceInstance.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "DeprovisionRequested",
		})
	}

	controllerutil.RemoveFinalizer(serviceInstance, korifiv1alpha1.CFManagedServiceInstanceFinalizerName)
	log.V(1).Info("finalizer removed")

	return ctrl.Result{}, nil
}

func (r *Reconciler) deprovisionServiceInstance(
	ctx context.Context,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFServiceInstance")

	assets, err := r.assets.GetServiceInstanceAssets(ctx, serviceInstance)
	if err != nil {
		log.Error(err, "failed to get service instance assets")
		return
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, assets.ServiceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", assets.ServiceBroker.Name)
		return
	}

	var deprovisionResponse osbapi.ServiceInstanceOperationResponse
	deprovisionResponse, err = osbapiClient.Deprovision(ctx, osbapi.InstanceDeprovisionPayload{
		ID: serviceInstance.Name,
		InstanceDeprovisionRequest: osbapi.InstanceDeprovisionRequest{
			ServiceId: assets.ServiceOffering.Spec.BrokerCatalog.ID,
			PlanID:    assets.ServicePlan.Spec.BrokerCatalog.ID,
		},
	})
	if err != nil {
		log.Error(err, "failed to deprovision service instance")
		return
	}

	serviceInstance.Status.DeprovisionOperation = deprovisionResponse.Operation
}

func (r *Reconciler) getNamespace(ctx context.Context, namespaceName string) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %q: %w", namespaceName, err)
	}
	return namespace, nil
}

func isProvisionRequested(instance *korifiv1alpha1.CFServiceInstance) bool {
	return meta.IsStatusConditionTrue(instance.Status.Conditions, korifiv1alpha1.ProvisionRequestedCondition)
}

func isDeprovisionRequested(instance *korifiv1alpha1.CFServiceInstance) bool {
	return meta.IsStatusConditionTrue(instance.Status.Conditions, korifiv1alpha1.DeprovisionRequestedCondition)
}

func isFailed(instance *korifiv1alpha1.CFServiceInstance) bool {
	return meta.IsStatusConditionTrue(instance.Status.Conditions, korifiv1alpha1.ProvisioningFailedCondition)
}

func isReady(instance *korifiv1alpha1.CFServiceInstance) bool {
	return meta.IsStatusConditionTrue(instance.Status.Conditions, korifiv1alpha1.StatusConditionReady)
}
