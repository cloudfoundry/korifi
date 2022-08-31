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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

// CFOrgReconciler reconciles a CFOrg object
type CFOrgReconciler struct {
	client                    client.Client
	scheme                    *runtime.Scheme
	log                       logr.Logger
	packageRegistrySecretName string
}

func NewCFOrgReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, packageRegistrySecretName string) *CFOrgReconciler {
	return &CFOrgReconciler{
		client:                    client,
		scheme:                    scheme,
		log:                       log,
		packageRegistrySecretName: packageRegistrySecretName,
	}
}

const (
	StatusConditionReady = "Ready"
	orgFinalizerName     = "cfOrg.korifi.cloudfoundry.org"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cforgs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=create;patch;delete;get;list;watch

/* These rbac annotations are not used directly by this controller.
   However, the application's service account must have them to create roles and servicebindings for CF roles,
   since a service account cannot grant permission that it itself does not have */
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildreconcilerinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildreconcilerinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildreconcilerinfos/finalizers,verbs=update
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFOrg object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CFOrgReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfOrg := new(korifiv1alpha1.CFOrg)
	err := r.client.Get(ctx, req.NamespacedName, cfOrg)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, fmt.Sprintf("Error when trying to fetch CFOrg %s/%s", req.Namespace, req.Name))
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	readyCondition := getConditionOrSetAsUnknown(&cfOrg.Status.Conditions, korifiv1alpha1.ReadyConditionType)
	if readyCondition == metav1.ConditionUnknown {
		if err = r.client.Status().Update(ctx, cfOrg); err != nil {
			r.log.Error(err, fmt.Sprintf("Error when trying to set status conditions on CFOrg %s/%s", req.Namespace, req.Name))
			return ctrl.Result{}, err
		}
	}

	err = r.addFinalizer(ctx, cfOrg)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error adding finalizer on CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	if !cfOrg.GetDeletionTimestamp().IsZero() {
		return finalize(ctx, r.client, r.log, cfOrg, orgFinalizerName)
	}

	labels := map[string]string{korifiv1alpha1.OrgNameLabel: cfOrg.Spec.DisplayName}
	err = createOrPatchNamespace(ctx, r.client, r.log, cfOrg, labels)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error creating namespace for CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	namespace, ok := getNamespace(ctx, r.client, cfOrg.Name)
	if !ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = propagateSecrets(ctx, r.client, r.log, cfOrg, r.packageRegistrySecretName)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error propagating secrets into CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	err = reconcileRoleBindings(ctx, r.client, r.log, cfOrg)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error propagating role-bindings into CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	cfOrg.Status.GUID = namespace.Name
	err = updateStatus(ctx, r.client, cfOrg, metav1.ConditionTrue)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error updating status on CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFOrgReconciler) addFinalizer(ctx context.Context, cfOrg *korifiv1alpha1.CFOrg) error {
	if controllerutil.ContainsFinalizer(cfOrg, orgFinalizerName) {
		return nil
	}

	originalCFOrg := cfOrg.DeepCopy()
	controllerutil.AddFinalizer(cfOrg, orgFinalizerName)

	err := r.client.Patch(ctx, cfOrg, client.MergeFrom(originalCFOrg))
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error adding finalizer to CFOrg/%s", cfOrg.Name))
		return err
	}

	r.log.Info(fmt.Sprintf("Finalizer added to CFOrg/%s", cfOrg.Name))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFOrgReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFOrg{}).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFOrgRequests),
		).
		Watches(
			&source.Kind{Type: &rbacv1.RoleBinding{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFOrgRequests),
		).
		Complete(r)
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
