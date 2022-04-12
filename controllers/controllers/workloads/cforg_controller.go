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

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

// CFOrgReconciler reconciles a CFOrg object
type CFOrgReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

func NewCFOrgReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) *CFOrgReconciler {
	r := CFOrgReconciler{
		client: client,
		scheme: scheme,
		log:    log,
	}
	return &r
}

const (
	StatusConditionReady  = "Ready"
	OrgNameLabel          = "cloudfoundry.org/org-name"
	hierarchyMetadataName = "hierarchy"
	APIVersion            = "workloads.cloudfoundry.org/v1alpha1"
	Kind                  = "CFOrg"
)

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cforgs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cforgs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cforgs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=list;create;delete;watch
//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchyconfigurations,verbs=get;list;watch;update;patch

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
	cfOrg := new(workloadsv1alpha1.CFOrg)
	err := r.client.Get(ctx, req.NamespacedName, cfOrg)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to fetch CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cfOrg.ObjectMeta.DeletionTimestamp != nil && !cfOrg.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	var anchor v1alpha2.SubnamespaceAnchor
	err = r.client.Get(ctx, req.NamespacedName, &anchor)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			r.log.Error(err, fmt.Sprintf("Error getting SubnamespaceAnchor for CFOrg %s/%s", req.Namespace, req.Name))
			return ctrl.Result{}, err
		}

		anchor, err = r.createSubnamespaceAnchor(ctx, req, cfOrg)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Requeue to verify subnamespaceanchor is ready
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	if anchor.Status.State != v1alpha2.Ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	namespace, ok := r.getNamespace(ctx, cfOrg.ObjectMeta.Name)
	if !ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = setCascadingDelete(ctx, r.client, req.Name)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error updating HierarchyConfiguration for CFOrg %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	cfOrg.Status.GUID = namespace.Name
	setStatusConditionOnLocalCopy(&cfOrg.Status.Conditions, StatusConditionReady, metav1.ConditionTrue, StatusConditionReady, "")
	err = r.client.Status().Update(ctx, cfOrg)
	if err != nil {
		r.log.Error(err, "unable to update CFOrg status")
		r.log.Info(fmt.Sprintf("CFOrg status: %+v", cfOrg.Status))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFOrgReconciler) createSubnamespaceAnchor(ctx context.Context, req ctrl.Request, cfOrg *workloadsv1alpha1.CFOrg) (v1alpha2.SubnamespaceAnchor, error) {
	anchor := v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				OrgNameLabel: cfOrg.Spec.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: APIVersion,
					Kind:       Kind,
					Name:       cfOrg.Name,
					UID:        cfOrg.GetUID(),
				},
			},
		},
	}

	err := r.client.Create(ctx, &anchor)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error creating SubnamespaceAnchor for CFOrg %s/%s", req.Namespace, req.Name))
		return anchor, err
	}

	setStatusConditionOnLocalCopy(&cfOrg.Status.Conditions, StatusConditionReady, metav1.ConditionUnknown, StatusConditionReady, "")
	err = r.client.Status().Update(ctx, cfOrg)
	if err != nil {
		r.log.Error(err, "unable to update CFOrg status")
		r.log.Info(fmt.Sprintf("CFOrg status: %+v", cfOrg.Status))
		return anchor, err
	}
	return anchor, nil
}

func (r *CFOrgReconciler) getNamespace(ctx context.Context, namespaceName string) (*corev1.Namespace, bool) {
	namespace := new(corev1.Namespace)
	err := r.client.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
	if err != nil {
		return nil, false
	}
	return namespace, true
}

func setCascadingDelete(ctx context.Context, userClient client.Client, orgGUID string) error {
	oldHC := v1alpha2.HierarchyConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hierarchyMetadataName,
			Namespace: orgGUID,
		},
	}
	newHC := oldHC
	newHC.Spec.AllowCascadingDeletion = true

	if err := userClient.Patch(ctx, &newHC, client.MergeFrom(&oldHC)); err != nil {
		return fmt.Errorf("failed to update hierarchy configuration: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFOrgReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFOrg{}).
		Complete(r)
}
