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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"
	trinityv1alpha1 "github.tools.sap/neoCoreArchitecture/trinity-service-manager/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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

//+kubebuilder:rbac:groups=extensions.korifi.cloudfoundry.org,resources=cfserviceofferings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=extensions.korifi.cloudfoundry.org,resources=cfserviceplans,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances,verbs=get;list;watch;create;update;patch;delete

func (r *ManagedCFServiceInstanceReconciler) ReconcileResource(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ctrl.Result, error) {
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
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, btpServiceInstance, func() error {
		return controllerutil.SetOwnerReference(cfServiceInstance, btpServiceInstance, r.scheme)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ManagedCFServiceInstanceReconciler) getServicePlan(ctx context.Context, servicePlanGuid string) (trinityv1alpha1.CFServicePlan, error) {
	servicePlans := trinityv1alpha1.CFServicePlanList{}
	err := r.k8sClient.List(ctx, &servicePlans, client.InNamespace(r.rootNamespace), client.MatchingFields{shared.IndexServicePlanGUID: servicePlanGuid})
	if err != nil {
		return trinityv1alpha1.CFServicePlan{}, err
	}

	if len(servicePlans.Items) != 1 {
		return trinityv1alpha1.CFServicePlan{}, fmt.Errorf("found %d service plans for guid %q, expected one", len(servicePlans.Items), servicePlanGuid)
	}

	return servicePlans.Items[0], nil
}

func (r *ManagedCFServiceInstanceReconciler) getServiceOffering(ctx context.Context, serviceOfferingGuid string) (trinityv1alpha1.CFServiceOffering, error) {
	serviceOfferings := trinityv1alpha1.CFServiceOfferingList{}
	err := r.k8sClient.List(ctx, &serviceOfferings, client.InNamespace(r.rootNamespace), client.MatchingFields{shared.IndexServiceOfferingGUID: serviceOfferingGuid})
	if err != nil {
		return trinityv1alpha1.CFServiceOffering{}, err
	}

	if len(serviceOfferings.Items) != 1 {
		return trinityv1alpha1.CFServiceOffering{}, fmt.Errorf("found %d service offerings for guid %q, expected one", len(serviceOfferings.Items), serviceOfferingGuid)
	}

	return serviceOfferings.Items[0], nil
}

func (r *ManagedCFServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.isManaged))
}

func (r *ManagedCFServiceInstanceReconciler) isManaged(object client.Object) bool {
	serviceInstance, ok := object.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return true
	}

	return serviceInstance.Spec.Type == "managed"
}
