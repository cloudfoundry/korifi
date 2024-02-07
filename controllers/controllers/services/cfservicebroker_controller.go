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
	ServiceBrokerGUIDLabel   = "cfservicebroker"
	ServiceOfferingGUIDLabel = "cfserviceoffering"
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
	Id               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Metadata         map[string]interface{} `json:"metadata"`
	Free             bool                   `json:"free"`
	Bindable         bool                   `json:"bindable"`
	BindingRotatable bool                   `json:"binding_rotatable"`
	PlanUpdateable   bool                   `json:"plan_updateable"`
	Schemas          struct {
		ServiceInstance struct {
			Create InputParameterSchema `json:"create"`
			Update InputParameterSchema `json:"update"`
		} `json:"service_instance"`
		ServiceBinding struct {
			Create InputParameterSchema `json:"create"`
		} `json:"service_binding"`
	} `json:"schemas"`
}

type CatalogService struct {
	Id                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	Description          string                 `json:"description"`
	Bindable             bool                   `json:"bindable"`
	InstancesRetrievable bool                   `json:"instances_retrievable"`
	BindingsRetrievable  bool                   `json:"bindings_retrievable"`
	PlanUpdateable       bool                   `json:"plan_updateable"`
	AllowContextUpdates  bool                   `json:"allow_context_updates"`
	Tags                 []string               `json:"tags"`
	Metadata             map[string]interface{} `json:"metadata"`
	DashboardClient      struct {
		Id          string `json:"id"`
		Secret      string `json:"secret"`
		RedirectUri string `json:"redirect_url"`
	} `json:"dashboard_client"`
	Plans []CatalogPlan `json:"plans"`
}

type ServiceCatalogResponse struct {
	Services []CatalogService `json:"services"`
}

type ServiceBrokerClient struct {
	baseURL       string
	authorization string
	offerings     []*korifiv1alpha1.CFServiceOffering
	plans         []*korifiv1alpha1.CFServicePlan
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
	//response, err := client.Get(catalogUrl)
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

func (r *CFServiceBrokerReconciler) mapServiceToCFOffering(guid string, catalogService CatalogService, cfServiceBroker *korifiv1alpha1.CFServiceBroker) *korifiv1alpha1.CFServiceOffering {
	return &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
			Labels: map[string]string{
				ServiceBrokerGUIDLabel: cfServiceBroker.Name,
			},
		},
		Spec: korifiv1alpha1.CFServiceOfferingSpec{
			Id:                catalogService.Id,
			OfferingName:      catalogService.Name,
			Description:       catalogService.Description,
			Available:         true,
			Tags:              []string{},
			Requires:          []string{},
			CreationTimestamp: metav1.Now(),
			UpdatedTimestamp:  metav1.Now(),
			Shareable:         false,
			//DocumentationUrl:     string(catalogService.Metadata["DocumentationUrl"]),
			Bindable:             catalogService.Bindable,
			PlanUpdateable:       catalogService.PlanUpdateable,
			InstancesRetrievable: catalogService.InstancesRetrievable,
			BindingsRetrievable:  catalogService.BindingsRetrievable,
			AllowContextUpdates:  catalogService.AllowContextUpdates,
			CatalogId:            catalogService.Id,
			//Metadata:             &map[string]string{},
		},
	}
}

func (r *CFServiceBrokerReconciler) mapPlanToCFPlan(guid, offeringGUID string, catalogPlan CatalogPlan, cfServiceBroker *korifiv1alpha1.CFServiceBroker) *korifiv1alpha1.CFServicePlan {
	return &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
			Labels: map[string]string{
				ServiceBrokerGUIDLabel:   cfServiceBroker.Name,
				ServiceOfferingGUIDLabel: offeringGUID,
			},
		},
		Spec: korifiv1alpha1.CFServicePlanSpec{
			Id:                catalogPlan.Id,
			PlanName:          catalogPlan.Name,
			Description:       catalogPlan.Description,
			Available:         true,
			Free:              catalogPlan.Free,
			CreationTimestamp: metav1.Now(),
			UpdatedTimestamp:  metav1.Now(),
			Relationships: korifiv1alpha1.RelationshipsSpec{
				ServiceOfferingGUID: offeringGUID,
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

	err := r.k8sClient.List(ctx, cfOfferingsList, client.InNamespace(r.rootNamespace), client.MatchingLabels{ServiceBrokerGUIDLabel: cfServiceBroker.Name})
	if err != nil {
		log.Info("failed to list existing cfserviceofferings", "reason", err)
		return ctrl.Result{}, err
	}

	actualOfferingMap := make(map[string]*korifiv1alpha1.CFServiceOffering)
	for _, offering := range cfOfferingsList.Items {
		actualOfferingMap[offering.Spec.Id] = &offering
	}

	err = r.k8sClient.List(ctx, cfPlansList, client.InNamespace(r.rootNamespace), client.MatchingLabels{ServiceBrokerGUIDLabel: cfServiceBroker.Name})
	if err != nil {
		log.Info("failed to list existing cfserviceplans", "reason", err)
		return ctrl.Result{}, err
	}

	actualPlanMap := make(map[string]*korifiv1alpha1.CFServicePlan)
	for _, plan := range cfPlansList.Items {
		actualPlanMap[plan.Spec.Id] = &plan
	}

	catalogResponse, err := r.getCatalog(ctx, cfServiceBroker)
	if err != nil {
		log.Info("failed to get broker catalog", "reason", err)
		return ctrl.Result{}, err
	}

	for _, service := range catalogResponse.Services {
		offeringGUID := ""
		offering, found := actualOfferingMap[service.Id]
		if found {
			offeringGUID = offering.Name
			delete(actualOfferingMap, service.Id)
		} else {
			offeringGUID = uuid.NewString()
		}

		actualCFServiceOffering := korifiv1alpha1.CFServiceOffering{
			ObjectMeta: metav1.ObjectMeta{
				Name:      offeringGUID,
				Namespace: r.rootNamespace,
			},
		}

		log.Info("Creating service offering ", "offering", service.Id)
		cfServiceOffering := r.mapServiceToCFOffering(offeringGUID, service, cfServiceBroker)
		_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, &actualCFServiceOffering, func() error {
			actualCFServiceOffering.Labels = cfServiceOffering.Labels
			actualCFServiceOffering.Spec = cfServiceOffering.Spec
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
			} else {
				planGUID = uuid.NewString()
			}

			log.Info("Creating service plan ", "plan", catalogPlan.Id)
			actualCFServicePlan := korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planGUID,
					Namespace: r.rootNamespace,
				},
			}

			cfServicePlan := r.mapPlanToCFPlan(planGUID, offeringGUID, catalogPlan, cfServiceBroker)
			_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, &actualCFServicePlan, func() error {
				actualCFServicePlan.Labels = cfServicePlan.Labels
				actualCFServicePlan.Spec = cfServicePlan.Spec
				return nil
			})
			if err != nil {
				log.Info("failed to create/patch service plan", "reason", err)
				return ctrl.Result{}, err
			}
		} //plans
	}

	return ctrl.Result{}, nil
}

func (r *CFServiceBrokerReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBroker{})
}
