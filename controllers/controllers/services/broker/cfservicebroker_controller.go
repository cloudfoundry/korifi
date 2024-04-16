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

package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	requeueInterval = 5 * time.Second
	ReadyCondition  = "Ready"
)

// CFServiceBrokerReconciler reconciles a CFServiceInstance object
type CFServiceBrokerReconciler struct {
	rootNamespace string
	k8sClient     client.Client
	scheme        *runtime.Scheme
	brokerClient  BrokerClient
	log           logr.Logger
}

func toBrokerServiceOffering(catalogService Service) korifiv1alpha1.BrokerServiceOffering {
	var raw_metadata []byte
	if catalogService.Metadata != nil {
		raw_metadata, _ = json.Marshal(catalogService.Metadata)
	}

	var documentationUrl *string
	if u, ok := catalogService.Metadata["documentationUrl"]; ok {
		documentationUrl = tools.PtrTo(u.(string))
	}
	return korifiv1alpha1.BrokerServiceOffering{
		Name:             catalogService.Name,
		Description:      catalogService.Description,
		Tags:             catalogService.Tags,
		Requires:         catalogService.Requires,
		Shareable:        true,
		DocumentationURL: documentationUrl,
		Broker_catalog: korifiv1alpha1.ServiceBrokerCatalog{
			Id: catalogService.Id,
			Features: korifiv1alpha1.BrokerCatalogFeatures{
				Plan_updateable:       catalogService.PlanUpdateable,
				Bindable:              catalogService.Bindable,
				Instances_retrievable: catalogService.InstancesRetrievable,
				Bindings_retrievable:  catalogService.BindingsRetrievable,
				Allow_context_updates: catalogService.AllowContextUpdates,
			},
			Metadata: &runtime.RawExtension{
				Raw: raw_metadata,
			},
		},
	}
}

func toBrokerServicePlan(offeringGUID string, catalogPlan Plan) korifiv1alpha1.BrokerServicePlan {
	var raw_metadata []byte
	if catalogPlan.Metadata != nil {
		raw_metadata, _ = json.Marshal(catalogPlan.Metadata)
	}
	return korifiv1alpha1.BrokerServicePlan{
		Name:             catalogPlan.Name,
		Free:             catalogPlan.Free,
		Description:      catalogPlan.Description,
		Maintenance_info: korifiv1alpha1.ServicePlanMaintenanceInfo{},
		Broker_catalog: korifiv1alpha1.ServicePlanBrokerCatalog{
			Id: catalogPlan.Id,
			Metadata: &runtime.RawExtension{
				Raw: raw_metadata,
			},
			Features: korifiv1alpha1.ServicePlanFeatures{
				Plan_updateable: catalogPlan.PlanUpdateable,
				Bindable:        catalogPlan.Bindable,
			},
		},
		Schemas: catalogPlan.Schemas,
	}
}

func NewCFServiceBrokerReconciler(
	rootNamespace string,
	client client.Client,
	scheme *runtime.Scheme,
	brokerClient BrokerClient,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBroker, *korifiv1alpha1.CFServiceBroker] {
	serviceBrokerReconciler := CFServiceBrokerReconciler{rootNamespace: rootNamespace, k8sClient: client, scheme: scheme, log: log, brokerClient: brokerClient}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFServiceBroker, *korifiv1alpha1.CFServiceBroker](log, client, &serviceBrokerReconciler)
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebrokers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebrokers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebrokers/finalizers,verbs=update
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceofferings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceofferings/finalizers,verbs=update
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceplans,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceplans/finalizers,verbs=update

func (r *CFServiceBrokerReconciler) ReconcileResource(ctx context.Context, cfServiceBroker *korifiv1alpha1.CFServiceBroker) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Reconciling CFServiceBroker Name", "Name", cfServiceBroker.Spec.Name)

	catalog, err := r.brokerClient.GetCatalog(ctx, cfServiceBroker)
	if err != nil {
		log.Info("failed to get broker catalog", "reason", err)
		return brokerNotReady(cfServiceBroker, err)
	}

	for _, service := range catalog.Services {
		log.Info("Reconcile service offering ", "offering", service.Id, "broker", cfServiceBroker.Name)

		serviceOffering := &korifiv1alpha1.CFServiceOffering{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateBrokerObjectUid(cfServiceBroker, service.Id),
				Namespace: r.rootNamespace,
			},
		}
		_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, serviceOffering, func() error {
			if serviceOffering.Labels == nil {
				serviceOffering.Labels = map[string]string{}
			}
			serviceOffering.Labels[korifiv1alpha1.RelServiceBrokerLabel] = cfServiceBroker.Name
			serviceOffering.Spec.BrokerServiceOffering = toBrokerServiceOffering(service)
			return nil
		})
		if err != nil {
			log.Info("failed to create/patch service offering", "reason", err)
			return brokerNotReady(cfServiceBroker, err)
		}

		for _, plan := range service.Plans {
			log.Info("Reconcile service plan ", "plan", plan.Id, "broker", cfServiceBroker.Name)
			servicePlan := &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generateBrokerObjectUid(cfServiceBroker, plan.Id),
					Namespace: r.rootNamespace,
				},
			}

			_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, servicePlan, func() error {
				servicePlan.Spec.BrokerServicePlan = toBrokerServicePlan(serviceOffering.Name, plan)

				if servicePlan.Labels == nil {
					servicePlan.Labels = map[string]string{}
				}
				servicePlan.Labels[korifiv1alpha1.RelServiceBrokerLabel] = cfServiceBroker.Name
				servicePlan.Labels[korifiv1alpha1.RelServiceOfferingLabel] = serviceOffering.Name
				return nil
			})
			if err != nil {
				log.Info("failed to create/patch service plan", "reason", err)
				return brokerNotReady(cfServiceBroker, err)
			}
		}
	}

	meta.SetStatusCondition(&cfServiceBroker.Status.Conditions, metav1.Condition{
		Type:               ReadyCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		Message:            "Ready",
		ObservedGeneration: cfServiceBroker.Generation,
	})

	return ctrl.Result{}, nil
}

func (r *CFServiceBrokerReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBroker{})
}

func generateBrokerObjectUid(broker *korifiv1alpha1.CFServiceBroker, objectId string) string {
	return tools.UUID(fmt.Sprintf("%s::%s", broker.Name, objectId))
}

func brokerNotReady(broker *korifiv1alpha1.CFServiceBroker, err error) (ctrl.Result, error) {
	meta.SetStatusCondition(&broker.Status.Conditions, metav1.Condition{
		Type:               ReadyCondition,
		Status:             metav1.ConditionFalse,
		Reason:             "Failed",
		Message:            err.Error(),
		ObservedGeneration: broker.Generation,
	})

	return ctrl.Result{}, err
}
