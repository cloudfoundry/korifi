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

package controllers

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/contour-router/config"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"istio.io/api/networking/v1alpha3"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"

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
)

const (
	CFRouteFinalizerName = "cfRoute.korifi.cloudfoundry.org"
)

// CFRouteReconciler reconciles a CFRoute object to create Contour resources
type CFRouteReconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	controllerConfig *config.ContourRouterConfig
}

func NewCFRouteReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	controllerConfig *config.ContourRouterConfig,
) *k8s.PatchingReconciler[korifiv1alpha1.CFRoute, *korifiv1alpha1.CFRoute] {
	routeReconciler := CFRouteReconciler{
		client:           client,
		scheme:           scheme,
		log:              log,
		controllerConfig: controllerConfig,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFRoute, *korifiv1alpha1.CFRoute](log, client, &routeReconciler)
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfroutes/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfdomains,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices;gateways,verbs=get;list;watch;create;update;patch;delete

func (r *CFRouteReconciler) ReconcileResource(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) (ctrl.Result, error) {
	// if !cfRoute.GetDeletionTimestamp().IsZero() {
	// 	return r.finalizeCFRoute(ctx, cfRoute)
	// }

	var cfDomain korifiv1alpha1.CFDomain
	err := r.client.Get(ctx, types.NamespacedName{Name: cfRoute.Spec.DomainRef.Name, Namespace: cfRoute.Spec.DomainRef.Namespace}, &cfDomain)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cfRoute.Status = createInvalidRouteStatus(cfRoute, "CFDomain not found", "InvalidDomainRef", err.Error())
			return ctrl.Result{}, err
		}
		cfRoute.Status = createInvalidRouteStatus(cfRoute, "Error fetching domain reference", "FetchDomainRef", err.Error())
		return ctrl.Result{}, err
	}

	// r.addFinalizer(ctx, cfRoute)

	err = r.createOrPatchServices(ctx, cfRoute)
	if err != nil {
		cfRoute.Status = createInvalidRouteStatus(cfRoute, "Error creating/patching services", "CreatePatchServices", err.Error())
		return ctrl.Result{}, err
	}

	if err := r.createOrPatchVirtualService(ctx, cfRoute, cfDomain); err != nil {
		cfRoute.Status = createInvalidRouteStatus(cfRoute, "Error creating/patching virtual service", "CreatePatchVirtualService", err.Error())
		return ctrl.Result{}, err
	}

	// if err := r.createOrPatchGateway(ctx, cfRoute, cfDomain); err != nil {
	// 	cfRoute.Status = createInvalidRouteStatus(cfRoute, "Error creating/patching gateway", "CreatePatchGateway", err.Error())
	// 	return ctrl.Result{}, err
	// }

	// err = r.createOrPatchRouteProxy(ctx, cfRoute)
	// if err != nil {
	// 	cfRoute.Status = createInvalidRouteStatus(cfRoute, "Error creating/patching Route Proxy", "CreatePatchRouteProxy", err.Error())
	// 	return ctrl.Result{}, err
	// }

	// err = r.createOrPatchFQDNProxy(ctx, cfRoute, &cfDomain)
	// if err != nil {
	// 	cfRoute.Status = createInvalidRouteStatus(cfRoute, "Error creating/patching FQDN Proxy", "CreatePatchFQDNProxy", err.Error())
	// 	return ctrl.Result{}, err
	// }

	err = r.deleteOrphanedServices(ctx, cfRoute)
	if err != nil {
		// technically, failing to delete the orphaned services does not make the CFRoute invalid so we don't mess with the cfRoute status here
		return ctrl.Result{}, err
	}

	cfRoute.Status = createValidRouteStatus(cfRoute, &cfDomain, "Valid CFRoute", "Valid", "Valid CFRoute")
	return ctrl.Result{}, nil
}

func createValidRouteStatus(cfRoute *korifiv1alpha1.CFRoute, cfDomain *korifiv1alpha1.CFDomain, description, reason, message string) korifiv1alpha1.CFRouteStatus {
	fqdn := cfRoute.Spec.Host + "." + cfDomain.Spec.Name
	cfRouteStatus := korifiv1alpha1.CFRouteStatus{
		FQDN:          fqdn,
		URI:           fqdn + cfRoute.Spec.Path,
		Destinations:  cfRoute.Spec.Destinations,
		CurrentStatus: korifiv1alpha1.ValidStatus,
		Description:   description,
		Conditions:    cfRoute.Status.Conditions,
	}

	meta.SetStatusCondition(&cfRouteStatus.Conditions, metav1.Condition{
		Type:    "Valid",
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	return cfRouteStatus
}

func createInvalidRouteStatus(cfRoute *korifiv1alpha1.CFRoute, description, reason, message string) korifiv1alpha1.CFRouteStatus {
	cfRouteStatus := korifiv1alpha1.CFRouteStatus{
		Description:   description,
		CurrentStatus: korifiv1alpha1.InvalidStatus,
		Conditions:    cfRoute.Status.Conditions,
	}

	meta.SetStatusCondition(&cfRouteStatus.Conditions, metav1.Condition{
		Type:    "Valid",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	return cfRouteStatus
}

func (r *CFRouteReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFRoute{})
}

// func (r *CFRouteReconciler) addFinalizer(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) {
// 	if controllerutil.ContainsFinalizer(cfRoute, CFRouteFinalizerName) {
// 		return
// 	}

// 	controllerutil.AddFinalizer(cfRoute, CFRouteFinalizerName)
// 	r.log.Info(fmt.Sprintf("Finalizer added to CFRoute/%s", cfRoute.Name))
// }

// func (r *CFRouteReconciler) finalizeCFRoute(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) (ctrl.Result, error) {
// 	r.log.Info(fmt.Sprintf("Reconciling deletion of CFRoute/%s", cfRoute.Name))

// 	if !controllerutil.ContainsFinalizer(cfRoute, CFRouteFinalizerName) {
// 		return ctrl.Result{}, nil
// 	}

// 	fqdnHTTPProxy, foundFQDNProxy, err := r.getFQDNProxy(ctx, cfRoute.Status.FQDN, cfRoute.Namespace, false)
// 	if err != nil {
// 		return ctrl.Result{}, err
// 	}

// 	// Cleanup the FQDN HTTPProxy on delete
// 	if foundFQDNProxy {
// 		err := r.finalizeFQDNProxy(ctx, cfRoute.Name, fqdnHTTPProxy)
// 		if err != nil {
// 			return ctrl.Result{}, err
// 		}
// 	}

// 	controllerutil.RemoveFinalizer(cfRoute, CFRouteFinalizerName)

// 	return ctrl.Result{}, nil
// }

// func (r *CFRouteReconciler) finalizeFQDNProxy(ctx context.Context, cfRouteName string, fqdnProxy *contourv1.HTTPProxy) error {
// 	return k8s.Patch(ctx, r.client, fqdnProxy, func() {
// 		var retainedIncludes []contourv1.Include
// 		for _, include := range fqdnProxy.Spec.Includes {
// 			if include.Name != cfRouteName {
// 				retainedIncludes = append(retainedIncludes, include)
// 			} else {
// 				r.log.Info(fmt.Sprintf("Removing sub-HTTPProxy for route %s from FQDN HTTPProxy", cfRouteName))
// 			}
// 		}
// 		fqdnProxy.Spec.Includes = retainedIncludes
// 	})
// }

func (r *CFRouteReconciler) createOrPatchServices(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	for i, destination := range cfRoute.Spec.Destinations {
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateServiceName(cfRoute.Spec.Destinations[i]),
				Namespace: cfRoute.Namespace,
			},
		}

		result, err := controllerutil.CreateOrPatch(ctx, r.client, service, func() error {
			service.Labels = map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:   destination.AppRef.Name,
				korifiv1alpha1.CFRouteGUIDLabelKey: cfRoute.Name,
			}

			err := controllerutil.SetOwnerReference(cfRoute, service, r.scheme)
			if err != nil {
				r.log.Error(err, "failed to set OwnerRef on Service")
				return err
			}

			service.Spec.Ports = []corev1.ServicePort{{
				Port: int32(destination.Port),
			}}
			service.Spec.Selector = map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:     destination.AppRef.Name,
				korifiv1alpha1.CFProcessTypeLabelKey: destination.ProcessType,
			}

			return nil
		})
		if err != nil {
			r.log.Error(err, fmt.Sprintf("failed to patch Service/%s", service.Name))
			return fmt.Errorf("service reconciliation failed for CFRoute/%s destinations", cfRoute.Name)
		}

		r.log.Info(fmt.Sprintf("Service/%s %s", service.Name, result))
	}

	return nil
}

func (r *CFRouteReconciler) createOrPatchVirtualService(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute, cfDomain korifiv1alpha1.CFDomain) error {
	fqdn := strings.ToLower(fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name))
	destinations := []*v1alpha3.HTTPRouteDestination{}
	for _, d := range cfRoute.Spec.Destinations {
		destinations = append(destinations, &v1alpha3.HTTPRouteDestination{
			Destination: &v1alpha3.Destination{
				Host: generateServiceName(d),
				Port: &v1alpha3.PortSelector{Number: uint32(d.Port)},
			},
		})
	}

	virtualService := &networkingv1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRoute.Name,
			Namespace: cfRoute.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.client, virtualService, func() error {
		virtualService.Spec.Hosts = []string{fqdn}
		virtualService.Spec.Gateways = []string{"korifi-api-system/korifi-app-gateway"}
		virtualService.Spec.Http = []*v1alpha3.HTTPRoute{{Route: destinations}}

		return nil
	})

	return err
}

func (r *CFRouteReconciler) createOrPatchGateway(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute, cfDomain korifiv1alpha1.CFDomain) error {
	fqdn := strings.ToLower(fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name))

	gateway := &networkingv1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRoute.Name,
			Namespace: cfRoute.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.client, gateway, func() error {
		gateway.Spec.Servers = []*v1alpha3.Server{{
			Port: &v1alpha3.Port{
				Number:   80,
				Protocol: "HTTP",
				Name:     "http",
			},
			Hosts: []string{fqdn},
		}}

		return nil
	})

	return err
}

func (r *CFRouteReconciler) createOrPatchRouteProxy(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	services := make([]contourv1.Service, 0, len(cfRoute.Spec.Destinations))

	for i, destination := range cfRoute.Spec.Destinations {
		services = append(services, contourv1.Service{
			Name: generateServiceName(cfRoute.Spec.Destinations[i]),
			Port: destination.Port,
		})
	}

	routeHTTPProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRoute.Name,
			Namespace: cfRoute.Namespace,
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, routeHTTPProxy, func() error {
		if len(services) == 0 {
			routeHTTPProxy.Spec.Routes = []contourv1.Route{}
		} else {
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

		err := controllerutil.SetOwnerReference(cfRoute, routeHTTPProxy, r.scheme)
		if err != nil {
			r.log.Error(err, "failed to set OwnerRef on route HTTPProxy")
			return err
		}

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to patch route HTTPProxy")
		return err
	}

	r.log.Info(fmt.Sprintf("Route HTTPProxy/%s %s", routeHTTPProxy.Name, result))
	return nil
}

func (r *CFRouteReconciler) createOrPatchFQDNProxy(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute, cfDomain *korifiv1alpha1.CFDomain) error {
	fqdn := strings.ToLower(fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name))

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

		err = controllerutil.SetOwnerReference(cfRoute, fqdnHTTPProxy, r.scheme)
		if err != nil {
			r.log.Error(err, "failed to set OwnerRef on FQDN HTTPProxy")
			return err
		}

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to patch FQDN HTTPProxy")
		return err
	}

	r.log.Info(fmt.Sprintf("FQDN HTTPProxy/%s %s", fqdnHTTPProxy.Name, result))
	return nil
}

func (r *CFRouteReconciler) getFQDNProxy(ctx context.Context, fqdn, namespace string, checkAllNamespaces bool) (*contourv1.HTTPProxy, bool, error) {
	var fqdnHTTPProxy contourv1.HTTPProxy

	var proxies contourv1.HTTPProxyList
	var listOptions client.ListOptions
	if !checkAllNamespaces {
		listOptions = client.ListOptions{Namespace: namespace}
	}

	err := r.client.List(ctx, &proxies, &listOptions)
	if err != nil {
		r.log.Error(err, "failed to list HTTPProxies")
		return nil, false, err
	}

	var found bool
	for _, proxy := range proxies.Items {
		if proxy.Spec.VirtualHost != nil && proxy.Spec.VirtualHost.Fqdn == fqdn {
			if found {
				err = fmt.Errorf("found multiple HTTPProxy with FQDN %s", fqdn)
				r.log.Error(err, "")
				return nil, false, err
			} else if proxy.Namespace != namespace {
				err = fmt.Errorf("found existing HTTPProxy with FQDN %s in another space", fqdn)
				r.log.Error(err, fmt.Sprintf("existing proxy found in namespace %s", proxy.Namespace))
				return nil, false, err
			}

			fqdnHTTPProxy = proxy
			found = true
		}
	}

	return &fqdnHTTPProxy, found, nil
}

func (r *CFRouteReconciler) deleteOrphanedServices(ctx context.Context, cfRoute *korifiv1alpha1.CFRoute) error {
	matchingLabelSet := map[string]string{
		korifiv1alpha1.CFRouteGUIDLabelKey: cfRoute.Name,
	}

	serviceList, err := r.fetchServicesByMatchingLabels(ctx, matchingLabelSet, cfRoute.Namespace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Failed to fetch services using label - %s : %s", korifiv1alpha1.CFRouteGUIDLabelKey, cfRoute.Name))
		return err
	}

	for i, service := range serviceList.Items {
		isOrphan := true
		for j := range cfRoute.Spec.Destinations {
			if service.Name == generateServiceName(cfRoute.Spec.Destinations[j]) {
				isOrphan = false
				break
			}
		}
		if isOrphan {
			err = r.client.Delete(ctx, &serviceList.Items[i])
			if err != nil {
				r.log.Error(err, fmt.Sprintf("failed to delete service %s", service.Name))
				return err
			}
		}
	}

	return nil
}

func (r *CFRouteReconciler) fetchServicesByMatchingLabels(ctx context.Context, labelSet map[string]string, namespace string) (*corev1.ServiceList, error) {
	selector, err := labels.ValidatedSelectorFromSet(labelSet)
	if err != nil {
		r.log.Error(err, "Error initializing label selector")
		return nil, err
	}

	serviceList := corev1.ServiceList{}
	err = r.client.List(ctx, &serviceList, &client.ListOptions{LabelSelector: selector, Namespace: namespace})
	if err != nil {
		r.log.Error(err, "Failed to list services")
		return nil, err
	}

	return &serviceList, nil
}

func generateServiceName(destination korifiv1alpha1.Destination) string {
	return fmt.Sprintf("s-%s", destination.GUID)
}
