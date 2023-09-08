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

package networking

import (
	"context"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
)

// CFRouteReconciler reconciles a CFRoute object to create Contour resources
type CFRouteReconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	controllerConfig *config.ControllerConfig
}

func NewCFRouteReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	controllerConfig *config.ControllerConfig,
) *k8s.PatchingReconciler[korifiv1alpha1.CFRoute, *korifiv1alpha1.CFRoute] {
	routeReconciler := CFRouteReconciler{client: client, scheme: scheme, log: log, controllerConfig: controllerConfig}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFRoute, *korifiv1alpha1.CFRoute](log, client, &routeReconciler)
}

func (r *CFRouteReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFRoute{}).
		Watches(
			&korifiv1alpha1.CFApp{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFAppRequests),
		)
}

func (r *CFRouteReconciler) enqueueCFAppRequests(ctx context.Context, o client.Object) []reconcile.Request {
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

//+kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies/status,verbs=get
//+kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *CFRouteReconciler) ReconcileResource(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	var err error

	if !cfRoute.GetDeletionTimestamp().IsZero() {
		err = r.finalizeCFRoute(ctx, cfRoute)
		if err != nil {
			log.Info("failed to finalize cf route", "reason", err)
		}
		return ctrl.Result{}, err
	}

	cfDomain := &korifiv1alpha1.CFDomain{}
	err = r.client.Get(ctx, types.NamespacedName{Name: cfRoute.Spec.DomainRef.Name, Namespace: cfRoute.Spec.DomainRef.Namespace}, cfDomain)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return setInvalidRouteStatus(log, cfRoute, "CFDomain not found", "InvalidDomainRef", err)
		}
		return setInvalidRouteStatus(log, cfRoute, "Error fetching domain reference", "FetchDomainRef", err)
	}

	effectiveDestinations, err := r.buildEffectiveDestinations(ctx, cfRoute)
	if err != nil {
		return setInvalidRouteStatus(log, cfRoute, "Error building effective destinations", "BuildEffectiveDestinations", err)
	}

	setValidRouteStatus(log, cfRoute, cfDomain, effectiveDestinations, "Valid CFRoute", "Valid", "Valid CFRoute")

	err = r.createOrPatchServices(ctx, cfRoute)
	if err != nil {
		return setInvalidRouteStatus(log, cfRoute, "Error creating/patching services", "CreatePatchServices", err)
	}

	err = r.createOrPatchRouteProxy(ctx, cfRoute)
	if err != nil {
		return setInvalidRouteStatus(log, cfRoute, "Error creating/patching Route Proxy", "CreatePatchRouteProxy", err)
	}

	err = r.createOrPatchFQDNProxy(ctx, cfRoute, cfDomain)
	if err != nil {
		return setInvalidRouteStatus(log, cfRoute, "Error creating/patching FQDN Proxy", "CreatePatchFQDNProxy", err)
	}

	err = r.deleteOrphanedServices(ctx, cfRoute)
	if err != nil {
		// technically, failing to delete the orphaned services does not make the CFRoute invalid so we don't mess with the cfRoute status here
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func setValidRouteStatus(
	log logr.Logger,
	cfRoute *korifiv1alpha1.CFRoute,
	cfDomain *korifiv1alpha1.CFDomain,
	destinations []korifiv1alpha1.Destination,
	description string,
	reason string,
	message string,
) {
	log.V(1).Info("set observed generation", "generation", cfRoute.Status.ObservedGeneration)

	fqdn := buildFQDN(cfRoute, cfDomain)
	cfRoute.Status.FQDN = fqdn
	cfRoute.Status.URI = fqdn + cfRoute.Spec.Path
	cfRoute.Status.Destinations = destinations
	cfRoute.Status.CurrentStatus = korifiv1alpha1.ValidStatus
	cfRoute.Status.Description = description
	cfRoute.Status.ObservedGeneration = cfRoute.Generation

	meta.SetStatusCondition(&cfRoute.Status.Conditions, metav1.Condition{
		Type:               "Valid",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cfRoute.Generation,
	})
}

func setInvalidRouteStatus(log logr.Logger, cfRoute *korifiv1alpha1.CFRoute, description, reason string, err error) (ctrl.Result, error) {
	log.V(1).Info("set observed generation", "generation", cfRoute.Status.ObservedGeneration)

	cfRoute.Status.CurrentStatus = korifiv1alpha1.InvalidStatus
	cfRoute.Status.Description = description
	cfRoute.Status.ObservedGeneration = cfRoute.Generation

	meta.SetStatusCondition(&cfRoute.Status.Conditions, metav1.Condition{
		Type:               "Valid",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		ObservedGeneration: cfRoute.Generation,
	})

	return ctrl.Result{}, err
}

func (r *CFRouteReconciler) finalizeCFRoute(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCRRoute")

	if !controllerutil.ContainsFinalizer(cfRoute, korifiv1alpha1.CFRouteFinalizerName) {
		return nil
	}

	if cfRoute.Status.FQDN != "" {
		fqdnHTTPProxy, foundFQDNProxy, err := r.getFQDNProxy(ctx, cfRoute.Status.FQDN, cfRoute.Namespace, false)
		if err != nil {
			return err
		}

		// Cleanup the FQDN HTTPProxy on delete
		if foundFQDNProxy {
			log.V(1).Info("found FQDN proxy", "fqdn", cfRoute.Status.FQDN)
			err := r.finalizeFQDNProxy(ctx, cfRoute.Name, fqdnHTTPProxy)
			if err != nil {
				return err
			}
		}
	}

	if controllerutil.RemoveFinalizer(cfRoute, korifiv1alpha1.CFRouteFinalizerName) {
		log.V(1).Info("finalizer removed")
	}

	return nil
}

func (r *CFRouteReconciler) finalizeFQDNProxy(ctx context.Context, cfRouteName string, fqdnProxy *contourv1.HTTPProxy) error {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeFQDNProxy")

	return k8s.PatchResource(ctx, r.client, fqdnProxy, func() {
		var retainedIncludes []contourv1.Include
		for _, include := range fqdnProxy.Spec.Includes {
			if include.Name != cfRouteName {
				retainedIncludes = append(retainedIncludes, include)
			} else {
				log.V(1).Info("removing sub-HTTPProxy from FQDN HTTPProxy", "removed name", include.Name)
			}
		}
		fqdnProxy.Spec.Includes = retainedIncludes
	})
}

func (r *CFRouteReconciler) createOrPatchServices(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchServices")

	for i, destination := range cfRoute.Status.Destinations {
		serviceName := generateServiceName(&cfRoute.Status.Destinations[i])
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

func (r *CFRouteReconciler) buildEffectiveDestinations(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) ([]korifiv1alpha1.Destination, error) {
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

			effectiveDest.Port = tools.PtrTo(8080)
			if len(droplet.Ports) > 0 {
				effectiveDest.Port = tools.PtrTo(int(droplet.Ports[0]))
			}
		}

		effectiveDestinations = append(effectiveDestinations, *effectiveDest)
	}

	return effectiveDestinations, nil
}

func (r *CFRouteReconciler) getAppCurrentDroplet(ctx context.Context, appNamespace, appName string) (*korifiv1alpha1.BuildDropletStatus, error) {
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

func (r *CFRouteReconciler) createOrPatchRouteProxy(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchRouteProxy").WithValues("httpProxyNamespace", cfRoute.Namespace, "httpProxyName", cfRoute.Name)

	services := []contourv1.Service{}

	for i, destination := range cfRoute.Status.Destinations {
		if destination.Port != nil {
			services = append(services, contourv1.Service{
				Name: generateServiceName(&cfRoute.Status.Destinations[i]),
				Port: *destination.Port,
			})
		}
	}

	routeHTTPProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRoute.Name,
			Namespace: cfRoute.Namespace,
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, routeHTTPProxy, func() error {
		routeHTTPProxy.Spec.Routes = []contourv1.Route{}

		if len(services) != 0 {
			routeHTTPProxy.Spec.Routes = []contourv1.Route{
				{
					Conditions: []contourv1.MatchCondition{
						{Prefix: cfRoute.Spec.Path},
					},
					Services:         services,
					EnableWebsockets: true,
				},
			}
		}

		err := controllerutil.SetControllerReference(cfRoute, routeHTTPProxy, r.scheme)
		if err != nil {
			log.Info("failed to set OwnerRef on route HTTPProxy", "reason", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Info("failed to patch route HTTPProxy", "reason", err)
		return err
	}

	log.V(1).Info("Route HTTPProxy reconciled", "operation", result)
	return nil
}

func (r *CFRouteReconciler) createOrPatchFQDNProxy(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute, cfDomain *korifiv1alpha1.CFDomain) error {
	fqdn := buildFQDN(cfRoute, cfDomain)

	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchFQDNProxy").WithValues("fqdn", fqdn)

	fqdnHTTPProxy, foundFQDNProxy, err := r.getFQDNProxy(ctx, fqdn, cfRoute.Namespace, true)
	if err != nil {
		return err
	}

	if !foundFQDNProxy {
		fqdnHTTPProxy = &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fqdn,
				Namespace: cfRoute.Namespace,
			},
		}
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, fqdnHTTPProxy, func() error {
		fqdnHTTPProxy.Spec.VirtualHost = &contourv1.VirtualHost{
			Fqdn: fqdn,
		}

		if tlsSecret := r.controllerConfig.WorkloadsTLSSecretNameWithNamespace(); tlsSecret != "" {
			fqdnHTTPProxy.Spec.VirtualHost.TLS = &contourv1.TLS{SecretName: tlsSecret}
		}

		routeAlreadyIncluded := false
		for _, include := range fqdnHTTPProxy.Spec.Includes {
			if include.Name == cfRoute.Name && include.Namespace == cfRoute.Namespace {
				routeAlreadyIncluded = true
			}
		}

		if !routeAlreadyIncluded {
			fqdnHTTPProxy.Spec.Includes = append(fqdnHTTPProxy.Spec.Includes, contourv1.Include{
				Name:      cfRoute.Name,
				Namespace: cfRoute.Namespace,
			})
		}

		// Cannot use SetControllerReference here as multiple CFRoutes can "own" the same FQDN HTTPProxy.
		err = controllerutil.SetOwnerReference(cfRoute, fqdnHTTPProxy, r.scheme)
		if err != nil {
			log.Info("failed to set OwnerRef on FQDN HTTPProxy", "reason", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Info("failed to patch FQDN HTTPProxy", "reason", err)
		return err
	}

	log.V(1).Info("FQDN HTTPProxy reconciled", "operation", result)
	return nil
}

func (r *CFRouteReconciler) getFQDNProxy(ctx context.Context, fqdn, namespace string, checkAllNamespaces bool) (*contourv1.HTTPProxy, bool, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("getFQDNProxy")

	var fqdnHTTPProxy contourv1.HTTPProxy

	var proxies contourv1.HTTPProxyList
	var listOptions client.ListOptions
	if !checkAllNamespaces {
		listOptions = client.ListOptions{Namespace: namespace}
	}

	err := r.client.List(ctx, &proxies, &listOptions)
	if err != nil {
		log.Info("failed to list HTTPProxies", "reason", err)
		return nil, false, err
	}

	var found bool
	for _, proxy := range proxies.Items {
		if proxy.Spec.VirtualHost != nil && proxy.Spec.VirtualHost.Fqdn == fqdn {
			if found {
				err = errors.New("duplicate HTTPProxy for FQDN")
				log.Info(err.Error())
				return nil, false, err
			} else if proxy.Namespace != namespace {
				err = errors.New("found existing HTTPProxy with same FQDN in another space")
				log.Info(err.Error(), "otherNamespace", proxy.Namespace)
				return nil, false, err
			}

			fqdnHTTPProxy = proxy
			found = true
		}
	}

	return &fqdnHTTPProxy, found, nil
}

func (r *CFRouteReconciler) deleteOrphanedServices(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
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
		for j := range cfRoute.Status.Destinations {
			if service.Name == generateServiceName(&cfRoute.Status.Destinations[j]) {
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

func (r *CFRouteReconciler) fetchServicesByMatchingLabels(ctx context.Context, labelSet map[string]string, namespace string) (*corev1.ServiceList, error) {
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

func generateServiceName(destination *korifiv1alpha1.Destination) string {
	return fmt.Sprintf("s-%s", destination.GUID)
}

func buildFQDN(cfRoute *korifiv1alpha1.CFRoute, cfDomain *korifiv1alpha1.CFDomain) string {
	return fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name)
}
