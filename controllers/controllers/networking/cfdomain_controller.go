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
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
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

type CFDomainReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

func NewCFDomainReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.CFDomain, *korifiv1alpha1.CFDomain] {
	routeReconciler := CFDomainReconciler{client: client, scheme: scheme, log: log}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFDomain, *korifiv1alpha1.CFDomain](log, client, &routeReconciler)
}

func (r *CFDomainReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFDomain{})
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfdomains,verbs=get;list;watch;patch;create;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfdomains/status,verbs=patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfdomains/finalizers,verbs=update

func (r *CFDomainReconciler) ReconcileResource(ctx context.Context, cfDomain *korifiv1alpha1.CFDomain) (ctrl.Result, error) {
	log := shared.ObjectLogger(r.log, cfDomain)
	ctx = logr.NewContext(ctx, log)

	if !cfDomain.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFDomain(ctx, cfDomain)
	}

	cfDomain.Status.ObservedGeneration = cfDomain.Generation
	log.V(1).Info("set observed generation", "generation", cfDomain.Status.ObservedGeneration)

	meta.SetStatusCondition(&cfDomain.Status.Conditions, metav1.Condition{
		Type:               "Valid",
		Status:             metav1.ConditionTrue,
		Reason:             "Valid",
		Message:            "Valid Domain",
		ObservedGeneration: cfDomain.Generation,
	})

	return ctrl.Result{}, nil
}

func (r *CFDomainReconciler) finalizeCFDomain(ctx context.Context, cfDomain *korifiv1alpha1.CFDomain) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFDomain")

	if !controllerutil.ContainsFinalizer(cfDomain, korifiv1alpha1.CFDomainFinalizerName) {
		return ctrl.Result{}, nil
	}

	domainRoutes, err := r.listRoutesForDomain(ctx, cfDomain)
	if err != nil {
		log.Info("failed to list CFRoutes", "reason", err)
		return ctrl.Result{}, err
	}

	if len(domainRoutes) == 0 {
		if controllerutil.RemoveFinalizer(cfDomain, korifiv1alpha1.CFDomainFinalizerName) {
			log.V(1).Info("finalizer removed")
		}

		return ctrl.Result{}, nil
	}

	for i := range domainRoutes {
		err = r.client.Delete(ctx, &domainRoutes[i])
		if err != nil {
			log.Info("failed to delete CFRoute", "reason", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Second}, nil
}

func (r *CFDomainReconciler) listRoutesForDomain(ctx context.Context, cfDomain *korifiv1alpha1.CFDomain) ([]korifiv1alpha1.CFRoute, error) {
	routesList := korifiv1alpha1.CFRouteList{}
	err := r.client.List(ctx, &routesList, client.MatchingFields{shared.IndexRouteDomainQualifiedName: cfDomain.Namespace + "." + cfDomain.Name})
	if err != nil {
		return nil, err
	}

	return routesList.Items, nil
}
