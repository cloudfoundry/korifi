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
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ServiceBrokerGUIDLabel = "cfservicebroker"
)

// CFServiceBrokerReconciler reconciles a CFServiceInstance object
type CFServiceBrokerReconciler struct {
	rootNamespace string
	k8sClient     client.Client
	scheme        *runtime.Scheme
	log           logr.Logger
}

type InputParameterSchema struct {
	Parameters map[string]interface{} `json:"parameters"`
}

type CatalogPlan struct {
	Id               string                             `json:"id"`
	Name             string                             `json:"name"`
	Description      string                             `json:"description"`
	Metadata         map[string]interface{}             `json:"metadata"`
	Free             bool                               `json:"free"`
	Bindable         bool                               `json:"bindable"`
	BindingRotatable bool                               `json:"binding_rotatable"`
	PlanUpdateable   bool                               `json:"plan_updateable"`
	Schemas          *korifiv1alpha1.ServicePlanSchemas `json:"schemas"`
}

type CatalogService struct {
	Id                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Bindable             bool     `json:"bindable"`
	InstancesRetrievable bool     `json:"instances_retrievable"`
	BindingsRetrievable  bool     `json:"bindings_retrievable"`
	PlanUpdateable       bool     `json:"plan_updateable"`
	AllowContextUpdates  bool     `json:"allow_context_updates"`
	Tags                 []string `json:"tags"`
	Requires             []string `json:"requires"`
	DocumentationUrl     *string  `json:"documentation_url"`
	// Shareable       bool                   `json:"shareable"`
	Metadata        map[string]interface{} `json:"metadata"`
	DashboardClient struct {
		Id          string `json:"id"`
		Secret      string `json:"secret"`
		RedirectUri string `json:"redirect_url"`
	} `json:"dashboard_client"`
	Plans []CatalogPlan `json:"plans"`
}

type ServiceCatalogResponse struct {
	Services []CatalogService `json:"services"`
}

func (r *CFServiceBrokerReconciler) getCatalog(ctx context.Context, cfServiceBroker *korifiv1alpha1.CFServiceBroker) (*ServiceCatalogResponse, error) {
	catalogResponse := &ServiceCatalogResponse{}

	secret := new(corev1.Secret)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBroker.Spec.SecretName, Namespace: r.rootNamespace}, secret)
	if err != nil {
		return catalogResponse, err
	}

	catalogUrl, err := url.JoinPath(cfServiceBroker.Spec.URL, "/v2/catalog")
	if err != nil {
		return catalogResponse, err
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	request, err := http.NewRequest("GET", catalogUrl, nil)
	if err != nil {
		return catalogResponse, err
	}

	authPlain := fmt.Sprintf("%s:%s", secret.Data["username"], secret.Data["password"])
	auth := base64.StdEncoding.EncodeToString([]byte(authPlain))
	request.Header.Add("Authorization", "Basic "+auth)
	response, err := client.Do(request)
	if err != nil {
		return catalogResponse, err
	}
	defer response.Body.Close()

	err = json.NewDecoder(response.Body).Decode(catalogResponse)
	if err != nil {
		return catalogResponse, err
	}

	return catalogResponse, nil
}

func mapCatalogToOffering(catalogService CatalogService, cfServiceBroker *korifiv1alpha1.CFServiceBroker) *korifiv1alpha1.BrokerServiceOffering {
	var raw_metadata []byte
	if catalogService.Metadata != nil {
		raw_metadata, _ = json.Marshal(catalogService.Metadata)
	}
	return &korifiv1alpha1.BrokerServiceOffering{
		Name:              catalogService.Name,
		Description:       catalogService.Description,
		Tags:              catalogService.Tags,
		Requires:          catalogService.Requires,
		Shareable:         true,
		Documentation_url: catalogService.DocumentationUrl,
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
		Relationships: korifiv1alpha1.ServiceOfferingRelationships{
			Service_broker: korifiv1alpha1.ToOneRelationship{
				Data: korifiv1alpha1.Relationship{
					GUID: cfServiceBroker.Name,
				},
			},
		},
	}
}

func mapCatalogToPlan(offeringGUID string, catalogPlan CatalogPlan, cfServiceBroker *korifiv1alpha1.CFServiceBroker) *korifiv1alpha1.BrokerServicePlan {
	var raw_metadata []byte
	if catalogPlan.Metadata != nil {
		raw_metadata, _ = json.Marshal(catalogPlan.Metadata)
	}
	return &korifiv1alpha1.BrokerServicePlan{
		Name:             catalogPlan.Name,
		Free:             catalogPlan.Free,
		Description:      &catalogPlan.Description,
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
		Relationships: korifiv1alpha1.ServicePlanRelationships{
			Service_offering: korifiv1alpha1.ToOneRelationship{
				Data: korifiv1alpha1.Relationship{
					GUID: offeringGUID,
				},
			},
		},
	}
}

func NewCFServiceBrokerReconciler(
	rootNamespace string,
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBroker, *korifiv1alpha1.CFServiceBroker] {
	serviceBrokerReconciler := CFServiceBrokerReconciler{rootNamespace: rootNamespace, k8sClient: client, scheme: scheme, log: log}
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

	cfOfferingsList := &korifiv1alpha1.CFServiceOfferingList{}
	cfPlansList := &korifiv1alpha1.CFServicePlanList{}

	err := r.k8sClient.List(ctx, cfOfferingsList, client.InNamespace(r.rootNamespace))
	if err != nil {
		log.Info("failed to list existing cfserviceofferings", "reason", err)
		return ctrl.Result{}, err
	}

	idToOfferingMap := make(map[string]*korifiv1alpha1.CFServiceOffering)
	guidToOfferingMap := make(map[string]*korifiv1alpha1.CFServiceOffering)
	for _, offering := range cfOfferingsList.Items {
		if offering.Spec.Relationships.Service_broker.Data.GUID == cfServiceBroker.Name {
			idToOfferingMap[offering.Spec.Broker_catalog.Id] = &offering
			guidToOfferingMap[offering.Name] = &offering
		}
	}

	err = r.k8sClient.List(ctx, cfPlansList, client.InNamespace(r.rootNamespace))
	if err != nil {
		log.Info("failed to list existing cfserviceplans", "reason", err)
		return ctrl.Result{}, err
	}

	actualPlanMap := make(map[string]*korifiv1alpha1.CFServicePlan)
	for _, plan := range cfPlansList.Items {
		_, serviceOfferingFound := guidToOfferingMap[plan.Spec.Relationships.Service_offering.Data.GUID]
		if serviceOfferingFound {
			actualPlanMap[plan.Spec.Broker_catalog.Id] = &plan
		}
	}

	catalogResponse, err := r.getCatalog(ctx, cfServiceBroker)
	if err != nil {
		log.Info("failed to get broker catalog", "reason", err)
		return ctrl.Result{}, err
	}

	for _, service := range catalogResponse.Services {
		offeringGUID := ""
		offering, found := idToOfferingMap[service.Id]
		if found {
			offeringGUID = offering.Name
			delete(idToOfferingMap, service.Id)
			log.Info("Found existing offering ", "offering", service.Id)
		} else {
			log.Info("No existing offering ", "offering", service.Id)
			offeringGUID = uuid.NewString()
		}

		actualCFServiceOffering := korifiv1alpha1.CFServiceOffering{
			ObjectMeta: metav1.ObjectMeta{
				Name:      offeringGUID,
				Namespace: r.rootNamespace,
			},
		}

		log.Info("Reconcile service offering ", "offering", service.Id)
		serviceOffering := mapCatalogToOffering(service, cfServiceBroker)
		_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, &actualCFServiceOffering, func() error {
			actualCFServiceOffering.Spec.ServiceOffering.BrokerServiceOffering = *serviceOffering
			return nil
		})
		if err != nil {
			log.Info("failed to create/patch service offering", "reason", err)
			return ctrl.Result{}, err
		}

		for _, catalogPlan := range service.Plans {
			planGUID := ""
			plan, found := actualPlanMap[catalogPlan.Id]
			if found {
				planGUID = plan.Name
				delete(actualPlanMap, catalogPlan.Id)
				log.Info("Found existing plan ", "plan", catalogPlan.Id)
			} else {
				log.Info("No existing plan ", "plan", catalogPlan.Id)
				planGUID = uuid.NewString()
			}

			log.Info("Reconcile service plan ", "plan", catalogPlan.Id)
			actualCFServicePlan := korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planGUID,
					Namespace: r.rootNamespace,
				},
			}

			servicePlan := mapCatalogToPlan(offeringGUID, catalogPlan, cfServiceBroker)
			_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, &actualCFServicePlan, func() error {
				actualCFServicePlan.Spec.ServicePlan.BrokerServicePlan = *servicePlan

				if actualCFServicePlan.Labels == nil {
					actualCFServicePlan.Labels = map[string]string{}
				}
				actualCFServicePlan.Labels[ServiceBrokerGUIDLabel] = cfServiceBroker.Name
				return nil
			})
			if err != nil {
				log.Info("failed to create/patch service plan", "reason", err)
				return ctrl.Result{}, err
			}
		} // plans
	}

	return ctrl.Result{}, nil
}

func (r *CFServiceBrokerReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBroker{})
}
