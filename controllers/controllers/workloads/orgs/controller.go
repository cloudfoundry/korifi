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

package orgs

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/k8sns"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	client              client.Client
	namespaceReconciler *k8sns.Reconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]
}

func NewReconciler(
	client client.Client,
	log logr.Logger,
	containerRegistrySecretNames []string,
	labelCompiler labels.Compiler,
) *k8s.PatchingReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg] {
	namespaceController := k8sns.NewReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](
		client,
		k8sns.NewNamespaceFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](
			client,
			&k8sns.NoopFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]{},
			korifiv1alpha1.CFOrgFinalizerName,
		),
		&cfOrgMetadataCompiler{
			labelCompiler: labelCompiler,
		},
		containerRegistrySecretNames,
	)

	return k8s.NewPatchingReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](log, client, &Reconciler{
		client:              client,
		namespaceReconciler: namespaceController,
	})
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFOrg{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFOrgRequests),
		).
		Watches(
			&rbacv1.RoleBinding{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFOrgRequests),
		)
}

func (r *Reconciler) enqueueCFOrgRequests(ctx context.Context, object client.Object) []reconcile.Request {
	cfOrgList := &korifiv1alpha1.CFOrgList{}
	err := r.client.List(ctx, cfOrgList, client.InNamespace(object.GetNamespace()))
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

func (r *Reconciler) ReconcileResource(ctx context.Context, cfOrg *korifiv1alpha1.CFOrg) (ctrl.Result, error) {
	var err error
	readyConditionBuilder := k8s.NewReadyConditionBuilder(cfOrg)
	defer func() {
		meta.SetStatusCondition(&cfOrg.Status.Conditions, readyConditionBuilder.WithError(err).Build())
	}()

	nsReconcileResult, err := r.namespaceReconciler.ReconcileResource(ctx, cfOrg)
	if (nsReconcileResult != ctrl.Result{}) || (err != nil) {
		return nsReconcileResult, err
	}

	readyConditionBuilder.Ready()
	return ctrl.Result{}, nil
}

type cfOrgMetadataCompiler struct {
	labelCompiler labels.Compiler
}

func (c *cfOrgMetadataCompiler) CompileLabels(cfOrg *korifiv1alpha1.CFOrg) map[string]string {
	return c.labelCompiler.Compile(map[string]string{
		korifiv1alpha1.OrgNameKey: korifiv1alpha1.OrgSpaceDeprecatedName,
		korifiv1alpha1.OrgGUIDKey: cfOrg.Name,
	})
}

func (c *cfOrgMetadataCompiler) CompileAnnotations(cfOrg *korifiv1alpha1.CFOrg) map[string]string {
	return map[string]string{
		korifiv1alpha1.OrgNameKey: cfOrg.Spec.DisplayName,
	}
}
