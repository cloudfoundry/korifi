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

	"github.com/go-logr/logr"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildReconcilerInfoName = "kpack-image-builder"
	ReadyConditionType      = "Ready"
)

func NewBuildReconcilerInfoReconciler(c client.Client, scheme *runtime.Scheme, log logr.Logger, clusterBuilderName, rootNamespaceName string) *BuildReconcilerInfoReconciler {
	return &BuildReconcilerInfoReconciler{
		Client:             c,
		Scheme:             scheme,
		Log:                log,
		ClusterBuilderName: clusterBuilderName,
		RootNamespaceName:  rootNamespaceName,
	}
}

// BuildReconcilerInfoReconciler reconciles a BuildReconcilerInfo object
type BuildReconcilerInfoReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Log                logr.Logger
	ClusterBuilderName string
	RootNamespaceName  string
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildreconcilerinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildreconcilerinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildreconcilerinfos/finalizers,verbs=update

//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders,verbs=get;list;watch
//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BuildReconcilerInfo object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *BuildReconcilerInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != BuildReconcilerInfoName || req.Namespace != r.RootNamespaceName {
		return ctrl.Result{}, nil
	}

	info := new(v1alpha1.BuildReconcilerInfo)
	err := r.Client.Get(ctx, req.NamespacedName, info)
	if err != nil {
		r.Log.Error(err, "Error when fetching BuildReconcilerInfo")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	clusterBuilder := new(buildv1alpha2.ClusterBuilder)
	err = r.Client.Get(ctx, types.NamespacedName{Name: r.ClusterBuilderName}, clusterBuilder)
	if err != nil {
		r.Log.Error(err, "Error when fetching ClusterBuilder")

		info.Status.Stacks = []v1alpha1.BuildReconcilerInfoStatusStack{}
		info.Status.Buildpacks = []v1alpha1.BuildReconcilerInfoStatusBuildpack{}
		meta.SetStatusCondition(&info.Status.Conditions, metav1.Condition{
			Type:    ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "cluster_builder_missing",
			Message: fmt.Sprintf("Error fetching ClusterBuilder %q: %q", r.ClusterBuilderName, err),
		})
		err = r.Client.Status().Update(ctx, info)
		if err != nil {
			r.Log.Error(err, "Error when updating BuildReconcilerInfo.status for failure")
		}
		return ctrl.Result{}, err
	}

	updatedTimestamp := lastUpdatedTime(clusterBuilder.ObjectMeta)
	info.Status.Stacks = clusterBuilderToStacks(clusterBuilder, updatedTimestamp)
	info.Status.Buildpacks = clusterBuilderToBuildpacks(clusterBuilder, updatedTimestamp)
	meta.SetStatusCondition(&info.Status.Conditions, metav1.Condition{
		Type:    ReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "cluster_builder_exists",
		Message: fmt.Sprintf("Found ClusterBuilder %q", r.ClusterBuilderName),
	})
	err = r.Client.Status().Update(ctx, info)
	if err != nil {
		r.Log.Error(err, "Error when updating BuildReconcilerInfo.status for success")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func clusterBuilderToStacks(clusterBuilder *buildv1alpha2.ClusterBuilder, updatedTimestamp metav1.Time) []v1alpha1.BuildReconcilerInfoStatusStack {
	if clusterBuilder.Status.Stack.ID == "" {
		return []v1alpha1.BuildReconcilerInfoStatusStack{}
	}

	return []v1alpha1.BuildReconcilerInfoStatusStack{
		{
			Name:              clusterBuilder.Status.Stack.ID,
			Description:       "",
			CreationTimestamp: clusterBuilder.CreationTimestamp,
			UpdatedTimestamp:  updatedTimestamp,
		},
	}
}

func clusterBuilderToBuildpacks(builder *buildv1alpha2.ClusterBuilder, updatedTimestamp metav1.Time) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
	buildpackRecords := make([]v1alpha1.BuildReconcilerInfoStatusBuildpack, 0, len(builder.Status.Order))
	for _, orderEntry := range builder.Status.Order {
		buildpackRecords = append(buildpackRecords, v1alpha1.BuildReconcilerInfoStatusBuildpack{
			Name:              orderEntry.Group[0].Id,
			Stack:             builder.Status.Stack.ID,
			Version:           orderEntry.Group[0].Version,
			CreationTimestamp: builder.CreationTimestamp,
			UpdatedTimestamp:  updatedTimestamp,
		})
	}
	return buildpackRecords
}

func lastUpdatedTime(metadata metav1.ObjectMeta) metav1.Time {
	latestTime := metadata.CreationTimestamp
	for _, managedField := range metadata.ManagedFields {
		t := managedField.Time
		if t != nil && t.After(latestTime.Time) {
			latestTime = *t
		}
	}
	return latestTime
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconcilerInfoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BuildReconcilerInfo{}).
		Watches(
			&source.Kind{Type: new(buildv1alpha2.ClusterBuilder)},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				if obj.GetName() == r.ClusterBuilderName {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      BuildReconcilerInfoName,
							Namespace: r.RootNamespaceName,
						},
					})
				}
				return requests
			})).
		Complete(r)
}
