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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SpaceNameLabel = "cloudfoundry.org/space-name"
)

// CFSpaceReconciler reconciles a CFSpace object
type CFSpaceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

func NewCFSpaceReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) *CFSpaceReconciler {
	return &CFSpaceReconciler{
		client: client,
		scheme: scheme,
		log:    log,
	}
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfspaces/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFSpace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CFSpaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfSpace := new(workloadsv1alpha1.CFSpace)
	err := r.client.Get(ctx, req.NamespacedName, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to fetch CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var anchor v1alpha2.SubnamespaceAnchor
	err = r.client.Get(ctx, req.NamespacedName, &anchor)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			r.log.Error(err, fmt.Sprintf("Error getting SubnamespaceAnchor for CFSpace %s/%s", req.Namespace, req.Name))
			return ctrl.Result{}, err
		}

		anchorLabels := map[string]string{
			SpaceNameLabel: cfSpace.Spec.Name,
		}
		anchor, err = createSubnamespaceAnchor(ctx, r.client, req, cfSpace, anchorLabels)
		if err != nil {
			r.log.Error(err, fmt.Sprintf("Error creating SubnamespaceAnchor for CFSpace %s/%s", req.Namespace, req.Name))
			return ctrl.Result{}, err
		}
		err = updateStatus(ctx, r.client, cfSpace, metav1.ConditionUnknown)
		if err != nil {
			r.log.Error(err, "unable to update CFSpace status")
			return ctrl.Result{}, err
		}

		// Requeue to verify subnamespaceanchor is ready
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	if anchor.Status.State != v1alpha2.Ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	namespace, ok := getNamespace(ctx, r.client, cfSpace.Name)
	if !ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	cfSpace.Status.GUID = namespace.Name
	err = updateStatus(ctx, r.client, cfSpace, metav1.ConditionTrue)
	if err != nil {
		r.log.Error(err, "unable to update CFSpace status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFSpaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFSpace{}).
		Complete(r)
}
