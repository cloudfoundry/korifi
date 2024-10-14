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
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

//counterfeiter:generate -o fake -fake-name BrokerClient code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi.BrokerClient

//counterfeiter:generate -o fake -fake-name BrokerClientFactory code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi.BrokerClientFactory

type Reconciler struct {
	k8sClient           client.Client
	osbapiClientFactory osbapi.BrokerClientFactory
	scheme              *runtime.Scheme
	rootNamespace       string
	log                 logr.Logger
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
	})
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		Named("managed-cfserviceinstance").
		WithEventFilter(predicate.NewPredicateFuncs(r.isManaged))
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

	servicePlan, err := r.getServicePlan(ctx, serviceInstance.Spec.PlanGUID)
	if err != nil {
		log.Error(err, "failed to get service plan")
		return ctrl.Result{}, err
	}

	serviceBroker, err := r.getServiceBroker(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service broker")
		return ctrl.Result{}, err
	}

	serviceOffering, err := r.getServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service offering")
		return ctrl.Result{}, err
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, serviceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", serviceBroker.Name)
		return ctrl.Result{}, fmt.Errorf("failed to create client for broker %q: %w", serviceBroker.Name, err)
	}

	if !isProvisionRequested(serviceInstance) {
		return r.provisionServiceInstance(ctx, osbapiClient, serviceInstance, servicePlan, serviceOffering)
	}

	lastOpResponse, err := osbapiClient.GetServiceInstanceLastOperation(ctx, osbapi.GetLastOperationPayload{
		ID: serviceInstance.Name,
		GetLastOperationRequest: osbapi.GetLastOperationRequest{
			ServiceId: serviceOffering.Spec.BrokerCatalog.ID,
			PlanID:    servicePlan.Spec.BrokerCatalog.ID,
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

func (r *Reconciler) provisionServiceInstance(
	ctx context.Context,
	osbapiClient osbapi.BrokerClient,
	serviceInstance *korifiv1alpha1.CFServiceInstance,
	servicePlan *korifiv1alpha1.CFServicePlan,
	serviceOffering *korifiv1alpha1.CFServiceOffering,
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
			ServiceId:  serviceOffering.Spec.BrokerCatalog.ID,
			PlanID:     servicePlan.Spec.BrokerCatalog.ID,
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
		err := r.deprovisionServiceInstance(ctx, serviceInstance)
		if err != nil {
			return ctrl.Result{}, err
		}

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

func (r *Reconciler) deprovisionServiceInstance(ctx context.Context, serviceInstance *korifiv1alpha1.CFServiceInstance) error {
	log := logr.FromContextOrDiscard(ctx).WithName("deprovisionServiceInstance")

	servicePlan, err := r.getServicePlan(ctx, serviceInstance.Spec.PlanGUID)
	if err != nil {
		log.Error(err, "failed to get service plan")
		return nil
	}

	serviceBroker, err := r.getServiceBroker(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service broker")
		return nil
	}

	serviceOffering, err := r.getServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service offering")
		return nil
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, serviceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", serviceBroker.Name)
		return nil
	}
	var deprovisionResponse osbapi.ServiceInstanceOperationResponse
	deprovisionResponse, err = osbapiClient.Deprovision(ctx, osbapi.InstanceDeprovisionPayload{
		ID: serviceInstance.Name,
		InstanceDeprovisionRequest: osbapi.InstanceDeprovisionRequest{
			ServiceId: serviceOffering.Spec.BrokerCatalog.ID,
			PlanID:    servicePlan.Spec.BrokerCatalog.ID,
		},
	})
	if err != nil {
		log.Error(err, "failed to deprovision service instance")
		return k8s.NewNotReadyError().WithReason("DeprovisionFailed")
	}

	serviceInstance.Status.DeprovisionOperation = deprovisionResponse.Operation
	return nil
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

func (r *Reconciler) getServiceOffering(ctx context.Context, offeringGUID string) (*korifiv1alpha1.CFServiceOffering, error) {
	serviceOffering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      offeringGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)
	if err != nil {
		return nil, fmt.Errorf("failed to get service offering %q: %w", offeringGUID, err)
	}

	return serviceOffering, nil
}

func (r *Reconciler) getServicePlan(ctx context.Context, planGUID string) (*korifiv1alpha1.CFServicePlan, error) {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)
	if err != nil {
		return nil, fmt.Errorf("failed to get service plan %q: %w", planGUID, err)
	}
	return servicePlan, nil
}

func (r *Reconciler) getServiceBroker(ctx context.Context, brokerGUID string) (*korifiv1alpha1.CFServiceBroker, error) {
	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      brokerGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)
	if err != nil {
		return nil, fmt.Errorf("failed to get service broker %q: %w", brokerGUID, err)
	}

	return serviceBroker, nil
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
