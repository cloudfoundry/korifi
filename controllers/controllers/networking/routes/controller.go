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

package routes

import (
	"context"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Reconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	controllerConfig *config.ControllerConfig
}

func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	controllerConfig *config.ControllerConfig,
) *k8s.PatchingReconciler[korifiv1alpha1.CFRoute, *korifiv1alpha1.CFRoute] {
	routeReconciler := Reconciler{client: client, scheme: scheme, log: log, controllerConfig: controllerConfig}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFRoute, *korifiv1alpha1.CFRoute](log, client, &routeReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFRoute{}).
		Watches(
			&korifiv1alpha1.CFApp{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFAppRequests),
		)
}

func (r *Reconciler) enqueueCFAppRequests(ctx context.Context, o client.Object) []reconcile.Request {
	var requests []reconcile.Request

	cfApp, ok := o.(*korifiv1alpha1.CFApp)
	if !ok {
		return []reconcile.Request{}
	}

	var appRoutes korifiv1alpha1.CFRouteList
	err := r.client.List(
		ctx,
		&appRoutes,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexRouteDestinationAppName: cfApp.Name},
	)
	if err != nil {
		return []reconcile.Request{}
	}

	for _, appRoute := range appRoutes.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      appRoute.Name,
				Namespace: appRoute.Namespace,
			},
		})
	}

	return requests
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) ReconcileResource(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("set observed generation", "generation", cfRoute.Status.ObservedGeneration)

	cfRoute.Status.ObservedGeneration = cfRoute.Generation

	if !cfRoute.GetDeletionTimestamp().IsZero() {
		err := r.finalizeCFRoute(ctx, cfRoute)
		if err != nil {
			log.Info("failed to finalize cf route", "reason", err)
		}
		return ctrl.Result{}, err
	}

	cfDomain := &korifiv1alpha1.CFDomain{}
	err := r.client.Get(ctx, types.NamespacedName{Name: cfRoute.Spec.DomainRef.Name, Namespace: cfRoute.Spec.DomainRef.Namespace}, cfDomain)
	if err != nil {
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("InvalidDomainRef")
	}

	err = r.createOrPatchServices(ctx, cfRoute)
	if err != nil {
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("CreatePatchServices")
	}

	err = r.reconcileHTTPRoute(ctx, cfRoute, cfDomain)
	if err != nil {
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("ReconcileHTTPRoute")
	}

	fqdn := buildFQDN(cfRoute, cfDomain)
	cfRoute.Status.FQDN = fqdn
	cfRoute.Status.URI = fqdn + cfRoute.Spec.Path

	effectiveDestinations, err := r.buildEffectiveDestinations(ctx, cfRoute)
	if err != nil {
		return ctrl.Result{}, k8s.NewNotReadyError().WithCause(err).WithReason("BuildEffectiveDestinations")
	}
	cfRoute.Status.Destinations = effectiveDestinations

	if cleanupErr := r.deleteOrphanedServices(ctx, cfRoute); cleanupErr != nil {
		// technically, failing to delete the orphaned services does not make
		// the CFRoute invalid or not ready so we don't mess with the cfRoute
		// ready status condition here
		return ctrl.Result{}, cleanupErr
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeCFRoute(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCRRoute")

	if !controllerutil.ContainsFinalizer(cfRoute, korifiv1alpha1.CFRouteFinalizerName) {
		return nil
	}

	if controllerutil.RemoveFinalizer(cfRoute, korifiv1alpha1.CFRouteFinalizerName) {
		log.V(1).Info("finalizer removed")
	}

	return nil
}

func (r *Reconciler) createOrPatchServices(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchServices")

	for _, destination := range cfRoute.Status.Destinations {
		serviceName := generateServiceName(destination)
		loopLog := log.WithValues("processType", destination.ProcessType, "appRef", destination.AppRef.Name, "serviceName", serviceName)

		if destination.Port == nil {
			continue
		}

		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: cfRoute.Namespace,
			},
		}

		result, err := controllerutil.CreateOrPatch(ctx, r.client, service, func() error {
			service.Labels = map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:   destination.AppRef.Name,
				korifiv1alpha1.CFRouteGUIDLabelKey: cfRoute.Name,
			}

			err := controllerutil.SetControllerReference(cfRoute, service, r.scheme)
			if err != nil {
				loopLog.Info("failed to set OwnerRef on Service", "reason", err)
				return err
			}

			service.Spec.Ports = []corev1.ServicePort{{
				Port: int32(*destination.Port),
			}}

			service.Spec.Selector = map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:     destination.AppRef.Name,
				korifiv1alpha1.CFProcessTypeLabelKey: destination.ProcessType,
			}

			return nil
		})
		if err != nil {
			log.Info("failed to patch Service", "reason", err)
			return fmt.Errorf("service reconciliation failed for CFRoute/%s destinations", cfRoute.Name)
		}

		log.V(1).Info("Service reconciled", "operation", result)
	}

	return nil
}

func (r *Reconciler) buildEffectiveDestinations(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) ([]korifiv1alpha1.Destination, error) {
	effectiveDestinations := []korifiv1alpha1.Destination{}

	for _, dest := range cfRoute.Spec.Destinations {
		effectiveDest := dest.DeepCopy()

		if effectiveDest.Protocol == nil {
			effectiveDest.Protocol = tools.PtrTo("http1")
		}

		if effectiveDest.Port == nil {
			droplet, err := r.getAppCurrentDroplet(ctx, cfRoute.Namespace, dest.AppRef.Name)
			if err != nil {
				return []korifiv1alpha1.Destination{}, err
			}

			if droplet == nil {
				continue
			}

			effectiveDest.Port = tools.PtrTo[int32](8080)
			if len(droplet.Ports) > 0 {
				effectiveDest.Port = tools.PtrTo(droplet.Ports[0])
			}
		}

		effectiveDestinations = append(effectiveDestinations, *effectiveDest)
	}

	return effectiveDestinations, nil
}

func (r *Reconciler) getAppCurrentDroplet(ctx context.Context, appNamespace, appName string) (*korifiv1alpha1.BuildDropletStatus, error) {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: appNamespace,
			Name:      appName,
		},
	}
	err := r.client.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)
	if err != nil {
		return nil, err
	}

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		return nil, nil
	}

	cfBuild := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfApp.Namespace,
			Name:      cfApp.Spec.CurrentDropletRef.Name,
		},
	}
	err = r.client.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)
	if err != nil {
		return nil, fmt.Errorf("failed to get build for app %q: %w", cfApp.Name, err)
	}

	return cfBuild.Status.Droplet, nil
}

func (r *Reconciler) reconcileHTTPRoute(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute, cfDomain *korifiv1alpha1.CFDomain) error {
	fqdn := buildFQDN(cfRoute, cfDomain)
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchHTTPRoute").WithValues("fqdn", fqdn, "path", cfRoute.Spec.Path)

	httpRoute := &gatewayv1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRoute.Name,
			Namespace: cfRoute.Namespace,
		},
	}

	if len(cfRoute.Status.Destinations) == 0 {
		err := r.client.Delete(ctx, httpRoute)
		if client.IgnoreNotFound(err) != nil {
			log.Info("failed to delete existing HTTPRoutes", "reason", err)
			return err
		}
		return nil
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, httpRoute, func() error {
		httpRoute.Spec.ParentRefs = []gatewayv1beta1.ParentReference{{
			Group:     tools.PtrTo(gatewayv1beta1.Group("gateway.networking.k8s.io")),
			Kind:      tools.PtrTo(gatewayv1beta1.Kind("Gateway")),
			Namespace: tools.PtrTo(gatewayv1beta1.Namespace(r.controllerConfig.Networking.GatewayNamespace)),
			Name:      gatewayv1beta1.ObjectName(r.controllerConfig.Networking.GatewayName),
		}}

		httpRoute.Spec.Hostnames = []gatewayv1beta1.Hostname{
			gatewayv1beta1.Hostname(fqdn),
		}

		httpRoute.Spec.Rules = []gatewayv1beta1.HTTPRouteRule{{
			BackendRefs: toBackendRefs(cfRoute.Status.Destinations),
		}}
		if cfRoute.Spec.Path != "" {
			httpRoute.Spec.Rules[0].Matches = []gatewayv1beta1.HTTPRouteMatch{{
				Path: &gatewayv1beta1.HTTPPathMatch{
					Type:  tools.PtrTo(gatewayv1.PathMatchPathPrefix),
					Value: tools.PtrTo(strings.ToLower(cfRoute.Spec.Path)),
				},
			}}
		}

		return controllerutil.SetControllerReference(cfRoute, httpRoute, r.scheme)
	})
	if err != nil {
		log.Info("failed to create/patch HTTPRoute", "reason", err)
		return err
	}

	log.V(1).Info("HTTPRoute reconciled", "operation", result)
	return nil
}

func (r *Reconciler) deleteOrphanedServices(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("deleteOrphanedServices")

	matchingLabelSet := map[string]string{
		korifiv1alpha1.CFRouteGUIDLabelKey: cfRoute.Name,
	}

	serviceList, err := r.fetchServicesByMatchingLabels(ctx, matchingLabelSet, cfRoute.Namespace)
	if err != nil {
		log.Info("failed to fetch services using label", "label", korifiv1alpha1.CFRouteGUIDLabelKey, "value", cfRoute.Name, "reason", err)
		return err
	}

	for i, service := range serviceList.Items {
		loopLog := log.WithValues("serviceName", service.Name)

		isOrphan := true
		for _, destination := range cfRoute.Status.Destinations {
			if service.Name == generateServiceName(destination) {
				isOrphan = false
				break
			}
		}

		if isOrphan {
			err = r.client.Delete(ctx, &serviceList.Items[i])
			if err != nil {
				loopLog.Info("failed to delete service", "reason", err)
				return err
			}
		}
	}

	return nil
}

func (r *Reconciler) fetchServicesByMatchingLabels(ctx context.Context, labelSet map[string]string, namespace string) (*corev1.ServiceList, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("fetchServicesByMatchingLabels")

	selector, err := labels.ValidatedSelectorFromSet(labelSet)
	if err != nil {
		log.Info("error initializing label selector", "reason", err)
		return nil, err
	}

	serviceList := corev1.ServiceList{}
	err = r.client.List(ctx, &serviceList, &client.ListOptions{LabelSelector: selector, Namespace: namespace})
	if err != nil {
		log.Info("failed to list services", "reason", err)
		return nil, err
	}

	return &serviceList, nil
}

func generateServiceName(destination korifiv1alpha1.Destination) string {
	return fmt.Sprintf("s-%s", destination.GUID)
}

func buildFQDN(cfRoute *korifiv1alpha1.CFRoute, cfDomain *korifiv1alpha1.CFDomain) string {
	return fmt.Sprintf("%s.%s", strings.ToLower(cfRoute.Spec.Host), cfDomain.Spec.Name)
}

func toBackendRefs(destinations []korifiv1alpha1.Destination) []gatewayv1beta1.HTTPBackendRef {
	backendRefs := []gatewayv1beta1.HTTPBackendRef{}

	for _, destination := range destinations {
		backendRefs = append(backendRefs, gatewayv1beta1.HTTPBackendRef{
			BackendRef: gatewayv1beta1.BackendRef{
				BackendObjectReference: gatewayv1beta1.BackendObjectReference{
					Kind: tools.PtrTo(gatewayv1beta1.Kind("Service")),
					Name: gatewayv1beta1.ObjectName(generateServiceName(destination)),
					Port: tools.PtrTo(gatewayv1beta1.PortNumber(*destination.Port)),
				},
			},
		})
	}

	return backendRefs
}
