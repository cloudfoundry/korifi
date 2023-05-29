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

package workloads

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	orgFinalizerName = "cfOrg.korifi.cloudfoundry.org"
)

// CFOrgReconciler reconciles a CFOrg object
type CFOrgReconciler struct {
	client                      client.Client
	scheme                      *runtime.Scheme
	log                         logr.Logger
	containerRegistrySecretName string
	labelCompiler               labels.Compiler
}

func NewCFOrgReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	containerRegistrySecretName string,
	labelCompiler labels.Compiler,
) *k8s.PatchingReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](log, client, &CFOrgReconciler{
		client:                      client,
		scheme:                      scheme,
		log:                         log,
		containerRegistrySecretName: containerRegistrySecretName,
		labelCompiler:               labelCompiler,
	})
}

func (r *CFOrgReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFOrg{}).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFOrgRequests),
		).
		Watches(
			&source.Kind{Type: &rbacv1.RoleBinding{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFOrgRequests),
		)
}

func (r *CFOrgReconciler) enqueueCFOrgRequests(object client.Object) []reconcile.Request {
	cfOrgList := &korifiv1alpha1.CFOrgList{}
	err := r.client.List(context.Background(), cfOrgList, client.InNamespace(object.GetNamespace()))
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(cfOrgList.Items))
	for i := range cfOrgList.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&cfOrgList.Items[i])}
	}

	return requests
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=create;patch;delete;get;list;watch

/* These rbac annotations are not used directly by this controller.
   However, the application's service account must have them to create roles and servicebindings for CF roles,
   since a service account cannot grant permission that it itself does not have */
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos/finalizers,verbs=update
//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders,verbs=get;list;watch
//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders/status,verbs=get
//+kubebuilder:rbac:groups="",resources=events,verbs=create;update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;patch
//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get
//+kubebuilder:rbac:groups="",resources=secrets,verbs=create;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=create;patch
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=create;delete;deletecollection
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads/status,verbs=patch
//+kubebuilder:rbac:groups="metrics.k8s.io",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="policy",resources=poddisruptionbudgets,verbs=create;deletecollection
//+kubebuilder:rbac:groups="policy",resources=podsecuritypolicies,verbs=use

func (r *CFOrgReconciler) ReconcileResource(ctx context.Context, cfOrg *korifiv1alpha1.CFOrg) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", cfOrg.Namespace, "name", cfOrg.Name)

	cfOrg.Status.ObservedGeneration = cfOrg.Generation
	log.V(1).Info("set observed generation", "generation", cfOrg.Status.ObservedGeneration)

	if !cfOrg.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, log, cfOrg)
	}

	shared.GetConditionOrSetAsUnknown(&cfOrg.Status.Conditions, korifiv1alpha1.ReadyConditionType, cfOrg.Generation)

	if controllerutil.AddFinalizer(cfOrg, orgFinalizerName) {
		log.Info("added finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	cfOrg.Status.GUID = cfOrg.Name

	err := createOrPatchNamespace(ctx, r.client, log, cfOrg, r.labelCompiler.Compile(map[string]string{
		korifiv1alpha1.OrgNameKey: korifiv1alpha1.OrgSpaceDeprecatedName,
		korifiv1alpha1.OrgGUIDKey: cfOrg.Name,
	}), map[string]string{
		korifiv1alpha1.OrgNameKey: cfOrg.Spec.DisplayName,
	})
	if err != nil {
		return logAndSetReadyStatus(fmt.Errorf("error creating namespace: %w", err), log, &cfOrg.Status.Conditions, "NamespaceCreation", cfOrg.Generation)
	}

	err = getNamespace(ctx, log, r.client, cfOrg.Name)
	if err != nil {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = propagateSecret(ctx, r.client, log, cfOrg, r.containerRegistrySecretName)
	if err != nil {
		return logAndSetReadyStatus(fmt.Errorf("error propagating secrets: %w", err), log, &cfOrg.Status.Conditions, "RegistrySecretPropagation", cfOrg.Generation)
	}

	err = reconcileRoleBindings(ctx, r.client, log, cfOrg)
	if err != nil {
		return logAndSetReadyStatus(fmt.Errorf("error propagating role-bindings: %w", err), log, &cfOrg.Status.Conditions, "RoleBindingPropagation", cfOrg.Generation)
	}

	meta.SetStatusCondition(&cfOrg.Status.Conditions, metav1.Condition{
		Type:               shared.StatusConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             shared.StatusConditionReady,
		ObservedGeneration: cfOrg.Generation,
	})

	return ctrl.Result{}, nil
}

func (r *CFOrgReconciler) finalize(ctx context.Context, log logr.Logger, org client.Object) (ctrl.Result, error) {
	log = log.WithName("finalize")

	if !controllerutil.ContainsFinalizer(org, orgFinalizerName) {
		return ctrl.Result{}, nil
	}

	err := r.client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org.GetName()}})
	if k8serrors.IsNotFound(err) {
		if controllerutil.RemoveFinalizer(org, orgFinalizerName) {
			log.V(1).Info("finalizer removed")
		}

		return ctrl.Result{}, nil
	}

	if err != nil {
		log.Info("failed to delete namespace", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}
