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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	BuilderInfoName = "kpack-image-builder"
)

func NewBuilderInfoReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	clusterBuilderName string,
	rootNamespaceName string,
) *k8s.PatchingReconciler[korifiv1alpha1.BuilderInfo] {
	builderInfoReconciler := BuilderInfoReconciler{
		k8sClient:          c,
		scheme:             scheme,
		log:                log,
		clusterBuilderName: clusterBuilderName,
		rootNamespaceName:  rootNamespaceName,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.BuilderInfo](log, c, &builderInfoReconciler)
}

type BuilderInfoReconciler struct {
	k8sClient          client.Client
	scheme             *runtime.Scheme
	log                logr.Logger
	clusterBuilderName string
	rootNamespaceName  string
}

func (r *BuilderInfoReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.BuilderInfo{}).
		Watches(
			new(buildv1alpha2.ClusterBuilder),
			handler.EnqueueRequestsFromMapFunc(r.enqueueBuilderInfoRequests),
		).
		WithEventFilter(predicate.NewPredicateFuncs(r.filterBuilderInfos))
}

func (r *BuilderInfoReconciler) enqueueBuilderInfoRequests(ctx context.Context, o client.Object) []reconcile.Request {
	var requests []reconcile.Request
	if o.GetName() == r.clusterBuilderName {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      BuilderInfoName,
				Namespace: r.rootNamespaceName,
			},
		})
	}
	return requests
}

func (r *BuilderInfoReconciler) filterBuilderInfos(object client.Object) bool {
	builderInfo, ok := object.(*korifiv1alpha1.BuilderInfo)
	if !ok {
		return true
	}

	return builderInfo.Name == BuilderInfoName && builderInfo.Namespace == r.rootNamespaceName
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=builderinfos/status,verbs=get;patch

//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders,verbs=get;list;watch
//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders/status,verbs=get

func (r *BuilderInfoReconciler) ReconcileResource(ctx context.Context, info *korifiv1alpha1.BuilderInfo) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	if !info.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	info.Status.ObservedGeneration = info.Generation
	log.V(1).Info("set observed generation", "generation", info.Status.ObservedGeneration)

	clusterBuilder := new(buildv1alpha2.ClusterBuilder)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: r.clusterBuilderName}, clusterBuilder)
	if err != nil {
		r.log.Info("error when fetching ClusterBuilder", "reason", err)

		info.Status.Stacks = []korifiv1alpha1.BuilderInfoStatusStack{}
		info.Status.Buildpacks = []korifiv1alpha1.BuilderInfoStatusBuildpack{}
		return ctrl.Result{}, k8s.NewNotReadyError().
			WithCause(err).
			WithReason("ClusterBuilderMissing").
			WithMessage(fmt.Sprintf("Error fetching ClusterBuilder %q: %s", r.clusterBuilderName, err))
	}

	updatedTimestamp := lastUpdatedTime(clusterBuilder.ObjectMeta)
	info.Status.Stacks = clusterBuilderToStacks(clusterBuilder, updatedTimestamp)
	info.Status.Buildpacks = clusterBuilderToBuildpacks(clusterBuilder, updatedTimestamp)

	clusterBuilderReadyCondition := clusterBuilder.Status.GetCondition(corev1alpha1.ConditionReady)
	if clusterBuilderReadyCondition == nil || clusterBuilderReadyCondition.Status != corev1.ConditionTrue {
		var msg string
		if clusterBuilderReadyCondition != nil {
			msg = clusterBuilderReadyCondition.Message
		}

		return ctrl.Result{}, k8s.NewNotReadyError().
			WithReason("ClusterBuilderNotReady").
			WithMessage(fmt.Sprintf("ClusterBuilder %q is not ready: %s", r.clusterBuilderName, msg))
	}

	return ctrl.Result{}, nil
}

func clusterBuilderToStacks(clusterBuilder *buildv1alpha2.ClusterBuilder, updatedTimestamp metav1.Time) []korifiv1alpha1.BuilderInfoStatusStack {
	if clusterBuilder.Status.Stack.ID == "" {
		return []korifiv1alpha1.BuilderInfoStatusStack{}
	}

	return []korifiv1alpha1.BuilderInfoStatusStack{
		{
			Name:              clusterBuilder.Status.Stack.ID,
			Description:       "",
			CreationTimestamp: clusterBuilder.CreationTimestamp,
			UpdatedTimestamp:  updatedTimestamp,
		},
	}
}

func clusterBuilderToBuildpacks(builder *buildv1alpha2.ClusterBuilder, updatedTimestamp metav1.Time) []korifiv1alpha1.BuilderInfoStatusBuildpack {
	buildpackRecords := make([]korifiv1alpha1.BuilderInfoStatusBuildpack, 0, len(builder.Status.Order))
	for _, orderEntry := range builder.Status.Order {
		buildpackRecords = append(buildpackRecords, korifiv1alpha1.BuilderInfoStatusBuildpack{
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
