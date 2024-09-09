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

package brokers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

//counterfeiter:generate -o fake -fake-name BrokerClient code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi.BrokerClient

//counterfeiter:generate -o fake -fake-name BrokerClientFactory code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi.BrokerClientFactory

type Reconciler struct {
	k8sClient           client.Client
	osbapiClientFactory osbapi.BrokerClientFactory
	scheme              *runtime.Scheme
	log                 logr.Logger
}

func NewReconciler(
	client client.Client,
	osbapiClientFactory osbapi.BrokerClientFactory,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBroker, *korifiv1alpha1.CFServiceBroker] {
	return k8s.NewPatchingReconciler(
		log,
		client,
		&Reconciler{
			k8sClient:           client,
			osbapiClientFactory: osbapiClientFactory,
			scheme:              scheme,
			log:                 log,
		},
	)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBroker{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.secretToServiceBroker),
		)
}

func (r *Reconciler) secretToServiceBroker(ctx context.Context, o client.Object) []reconcile.Request {
	serviceBrokers := korifiv1alpha1.CFServiceBrokerList{}
	if err := r.k8sClient.List(ctx, &serviceBrokers,
		client.InNamespace(o.GetNamespace()),
		client.MatchingFields{
			shared.IndexServiceBrokerCredentialsSecretName: o.GetName(),
		}); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, sb := range serviceBrokers.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sb.Name,
				Namespace: sb.Namespace,
			},
		})
	}

	return requests
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebrokers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebrokers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceofferings,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfserviceplans,verbs=get;list;watch;create;update;patch

func (r *Reconciler) ReconcileResource(ctx context.Context, cfServiceBroker *korifiv1alpha1.CFServiceBroker) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("broker-id", cfServiceBroker.Name)

	cfServiceBroker.Status.ObservedGeneration = cfServiceBroker.Generation
	log.V(1).Info("set observed generation", "generation", cfServiceBroker.Status.ObservedGeneration)

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBroker.Namespace,
			Name:      cfServiceBroker.Spec.Credentials.Name,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		notReadyErr := k8s.NewNotReadyError().WithCause(err).WithReason("CredentialsSecretNotAvailable")
		if apierrors.IsNotFound(err) {
			notReadyErr = notReadyErr.WithRequeueAfter(2 * time.Second)
		}
		return ctrl.Result{}, notReadyErr
	}

	log.V(1).Info("credentials secret", "name", credentialsSecret.Name, "version", credentialsSecret.ResourceVersion)
	cfServiceBroker.Status.CredentialsObservedVersion = credentialsSecret.ResourceVersion

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, cfServiceBroker)
	if err != nil {
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("OSBAPIClientCreationFailed")
	}

	catalog, err := osbapiClient.GetCatalog(ctx)
	if err != nil {
		log.Error(err, "failed to get catalog from broker", "broker", cfServiceBroker.Name)
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("GetCatalogFailed")
	}

	err = r.reconcileCatalog(ctx, cfServiceBroker, catalog)
	if err != nil {
		log.Error(err, "failed to reconcile catalog")
		return ctrl.Result{}, fmt.Errorf("failed to reconcile catalog: %v", err)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileCatalog(ctx context.Context, cfServiceBroker *korifiv1alpha1.CFServiceBroker, catalog osbapi.Catalog) error {
	for _, service := range catalog.Services {
		err := r.reconcileCatalogService(ctx, cfServiceBroker, service)
		if err != nil {
			return err
		}

	}
	return nil
}

func (r *Reconciler) reconcileCatalogService(ctx context.Context, cfServiceBroker *korifiv1alpha1.CFServiceBroker, catalogService osbapi.Service) error {
	serviceOffering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tools.NamespacedUUID(cfServiceBroker.Name, catalogService.ID),
			Namespace: cfServiceBroker.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, serviceOffering, func() error {
		if serviceOffering.Labels == nil {
			serviceOffering.Labels = map[string]string{}
		}
		serviceOffering.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] = cfServiceBroker.Name
		serviceOffering.Labels[korifiv1alpha1.RelServiceBrokerNameLabel] = cfServiceBroker.Spec.Name

		var err error
		serviceOffering.Spec.ServiceOffering, err = toSpecServiceOffering(catalogService)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile service offering %q : %w", catalogService.ID, err)
	}

	for _, catalogPlan := range catalogService.Plans {
		err = r.reconcileCatalogPlan(ctx, serviceOffering, catalogPlan)
		if err != nil {
			return fmt.Errorf("failed to reconcile service plan %q for service offering %q: %w", catalogPlan.ID, catalogService.ID, err)
		}
	}

	return nil
}

func (r *Reconciler) reconcileCatalogPlan(ctx context.Context, serviceOffering *korifiv1alpha1.CFServiceOffering, catalogPlan osbapi.Plan) error {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tools.NamespacedUUID(serviceOffering.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel], catalogPlan.ID),
			Namespace: serviceOffering.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, servicePlan, func() error {
		if servicePlan.Labels == nil {
			servicePlan.Labels = map[string]string{}
		}
		servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] = serviceOffering.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel]
		servicePlan.Labels[korifiv1alpha1.RelServiceBrokerNameLabel] = serviceOffering.Labels[korifiv1alpha1.RelServiceBrokerNameLabel]
		servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel] = serviceOffering.Name
		servicePlan.Labels[korifiv1alpha1.RelServiceOfferingNameLabel] = serviceOffering.Spec.Name

		rawMetadata, err := json.Marshal(catalogPlan.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal service plan %q metadata: %w", catalogPlan.ID, err)
		}

		visibilityType := korifiv1alpha1.AdminServicePlanVisibilityType
		if servicePlan.Spec.Visibility.Type != "" {
			visibilityType = servicePlan.Spec.Visibility.Type
		}

		servicePlan.Spec = korifiv1alpha1.CFServicePlanSpec{
			ServicePlan: services.ServicePlan{
				Name:        catalogPlan.Name,
				Free:        catalogPlan.Free,
				Description: catalogPlan.Description,
				BrokerCatalog: services.ServicePlanBrokerCatalog{
					ID: catalogPlan.ID,
					Metadata: &runtime.RawExtension{
						Raw: rawMetadata,
					},
					Features: services.ServicePlanFeatures{
						PlanUpdateable: catalogPlan.PlanUpdateable,
						Bindable:       catalogPlan.Bindable,
					},
				},
				Schemas: catalogPlan.Schemas,
			},
			Visibility: korifiv1alpha1.ServicePlanVisibility{
				Type: visibilityType,
			},
		}

		return nil
	})

	return err
}

func toSpecServiceOffering(catalogService osbapi.Service) (services.ServiceOffering, error) {
	offering := services.ServiceOffering{
		Name:        catalogService.Name,
		Description: catalogService.Description,
		Tags:        catalogService.Tags,
		Requires:    catalogService.Requires,
		BrokerCatalog: services.ServiceBrokerCatalog{
			ID:       catalogService.ID,
			Features: catalogService.BrokerCatalogFeatures,
		},
	}

	if catalogService.Metadata != nil {
		if u, ok := catalogService.Metadata["documentationUrl"]; ok {
			offering.DocumentationURL = tools.PtrTo(u.(string))
		}

		rawMd, err := json.Marshal(catalogService.Metadata)
		if err != nil {
			return services.ServiceOffering{}, err
		}
		offering.BrokerCatalog.Metadata = &runtime.RawExtension{
			Raw: rawMd,
		}

	}

	return offering, nil
}
