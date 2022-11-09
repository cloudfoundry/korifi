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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuilderInfoName    = "kpack-image-builder"
	ReadyConditionType = "Ready"
)

func NewBuilderInfoReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	clusterBuilderName string,
	rootNamespaceName string,
) *k8s.PatchingReconciler[v1alpha1.BuilderInfo, *v1alpha1.BuilderInfo] {
	builderInfoReconciler := BuilderInfoReconciler{
		k8sClient:          c,
		scheme:             scheme,
		log:                log,
		clusterBuilderName: clusterBuilderName,
		rootNamespaceName:  rootNamespaceName,
	}
	return k8s.NewPatchingReconciler[v1alpha1.BuilderInfo, *v1alpha1.BuilderInfo](log, c, &builderInfoReconciler)
}

// BuilderInfoReconciler reconciles a BuilderInfo object
type BuilderInfoReconciler struct {
	k8sClient          client.Client
	scheme             *runtime.Scheme
	log                logr.Logger
	clusterBuilderName string
	rootNamespaceName  string
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos/status,verbs=get;patch

//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders,verbs=get;list;watch
//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders/status,verbs=get

func (r *BuilderInfoReconciler) ReconcileResource(ctx context.Context, info *v1alpha1.BuilderInfo) (ctrl.Result, error) {
	clusterBuilder := new(buildv1alpha2.ClusterBuilder)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: r.clusterBuilderName}, clusterBuilder)
	if err != nil {
		r.log.Error(err, "Error when fetching ClusterBuilder")

		info.Status.Stacks = []v1alpha1.BuilderInfoStatusStack{}
		info.Status.Buildpacks = []v1alpha1.BuilderInfoStatusBuildpack{}
		meta.SetStatusCondition(&info.Status.Conditions, metav1.Condition{
			Type:    ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "cluster_builder_missing",
			Message: fmt.Sprintf("Error fetching ClusterBuilder %q: %q", r.clusterBuilderName, err),
		})
		return ctrl.Result{}, err
	}

	updatedTimestamp := lastUpdatedTime(clusterBuilder.ObjectMeta)
	info.Status.Stacks = clusterBuilderToStacks(clusterBuilder, updatedTimestamp)
	info.Status.Buildpacks = clusterBuilderToBuildpacks(clusterBuilder, updatedTimestamp)
	meta.SetStatusCondition(&info.Status.Conditions, metav1.Condition{
		Type:    ReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "cluster_builder_exists",
		Message: fmt.Sprintf("Found ClusterBuilder %q", r.clusterBuilderName),
	})

	return ctrl.Result{}, nil
}

func clusterBuilderToStacks(clusterBuilder *buildv1alpha2.ClusterBuilder, updatedTimestamp metav1.Time) []v1alpha1.BuilderInfoStatusStack {
	if clusterBuilder.Status.Stack.ID == "" {
		return []v1alpha1.BuilderInfoStatusStack{}
	}

	return []v1alpha1.BuilderInfoStatusStack{
		{
			Name:              clusterBuilder.Status.Stack.ID,
			Description:       "",
			CreationTimestamp: clusterBuilder.CreationTimestamp,
			UpdatedTimestamp:  updatedTimestamp,
		},
	}
}

func clusterBuilderToBuildpacks(builder *buildv1alpha2.ClusterBuilder, updatedTimestamp metav1.Time) []v1alpha1.BuilderInfoStatusBuildpack {
	buildpackRecords := make([]v1alpha1.BuilderInfoStatusBuildpack, 0, len(builder.Status.Order))
	for _, orderEntry := range builder.Status.Order {
		buildpackRecords = append(buildpackRecords, v1alpha1.BuilderInfoStatusBuildpack{
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

func (r *BuilderInfoReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BuilderInfo{}).
		Watches(
			&source.Kind{Type: new(buildv1alpha2.ClusterBuilder)},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				if obj.GetName() == r.clusterBuilderName {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      BuilderInfoName,
							Namespace: r.rootNamespaceName,
						},
					})
				}
				return requests
			})).
		WithEventFilter(predicate.NewPredicateFuncs(r.filterBuilderInfos))
}

func (r *BuilderInfoReconciler) filterBuilderInfos(object client.Object) bool {
	builderInfo, ok := object.(*v1alpha1.BuilderInfo)
	if !ok {
		return true
	}

	return builderInfo.Name == BuilderInfoName && builderInfo.Namespace == r.rootNamespaceName
}
