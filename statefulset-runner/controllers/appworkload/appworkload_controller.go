/*
Copyright 2022.

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

package appworkload

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/appworkload/state"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// Environment Variable Names
	EnvPodName              = "POD_NAME"
	EnvCFInstanceIP         = "CF_INSTANCE_IP"
	EnvCFInstanceGUID       = "CF_INSTANCE_GUID"
	EnvCFInstanceInternalIP = "CF_INSTANCE_INTERNAL_IP"
	EnvCFInstanceIndex      = "CF_INSTANCE_INDEX"
	EnvServiceBindingRoot   = "SERVICE_BINDING_ROOT"

	// StatefulSet Keys
	AnnotationVersion     = "korifi.cloudfoundry.org/version"
	AnnotationAppID       = "korifi.cloudfoundry.org/application-id"
	AnnotationProcessGUID = "korifi.cloudfoundry.org/process-guid"

	LabelVersion         = "korifi.cloudfoundry.org/version"
	LabelAppGUID         = "korifi.cloudfoundry.org/app-guid"
	LabelAppWorkloadGUID = "korifi.cloudfoundry.org/appworkload-guid"
	LabelProcessType     = "korifi.cloudfoundry.org/process-type"

	ApplicationContainerName = "application"
	ServiceAccountName       = "korifi-app"

	LivenessFailureThreshold  = 4
	ReadinessFailureThreshold = 1
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o ./fake -fake-name PDB . PDB
type PDB interface {
	Update(ctx context.Context, statefulSet *appsv1.StatefulSet) error
}

//counterfeiter:generate -o ./fake -fake-name WorkloadToStatefulsetConverter . WorkloadToStatefulsetConverter
type WorkloadToStatefulsetConverter interface {
	Convert(appWorkload *korifiv1alpha1.AppWorkload) (*appsv1.StatefulSet, error)
}

// AppWorkloadReconciler reconciles a AppWorkload object
type AppWorkloadReconciler struct {
	k8sClient        client.Client
	scheme           *runtime.Scheme
	workloadsToStSet WorkloadToStatefulsetConverter
	pdb              PDB
	log              logr.Logger
	stateCollector   *state.AppWorkloadStateCollector
}

func NewAppWorkloadReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	workloadsToStSet WorkloadToStatefulsetConverter,
	pdb PDB,
	log logr.Logger,
	stateCollector *state.AppWorkloadStateCollector,
) *k8s.PatchingReconciler[korifiv1alpha1.AppWorkload, *korifiv1alpha1.AppWorkload] {
	appWorkloadReconciler := AppWorkloadReconciler{
		k8sClient:        c,
		scheme:           scheme,
		workloadsToStSet: workloadsToStSet,
		pdb:              pdb,
		log:              log,
		stateCollector:   stateCollector,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.AppWorkload, *korifiv1alpha1.AppWorkload](log, c, &appWorkloadReconciler)
}

func (r *AppWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.AppWorkload{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&appsv1.StatefulSet{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueAppWorkloadRequests),
		).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueAppWorkloadRequests),
		).
		WithEventFilter(predicate.NewPredicateFuncs(filterAppWorkloads))
}

func (r *AppWorkloadReconciler) enqueueAppWorkloadRequests(ctx context.Context, o client.Object) []reconcile.Request {
	var requests []reconcile.Request

	if appWorkloadName, ok := o.GetLabels()[LabelAppWorkloadGUID]; ok {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      appWorkloadName,
				Namespace: o.GetNamespace(),
			},
		})
	}

	return requests
}

func filterAppWorkloads(object client.Object) bool {
	appWorkload, ok := object.(*korifiv1alpha1.AppWorkload)
	if !ok {
		return true
	}

	return appWorkload.Spec.RunnerName == controllers.AppWorkloadReconcilerName
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads/status,verbs=get;patch

//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;patch;get;list;watch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch

//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=create;patch;deletecollection

func (r *AppWorkloadReconciler) ReconcileResource(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	appWorkload.Status.ObservedGeneration = appWorkload.Generation
	log.V(1).Info("set observed generation", "generation", appWorkload.Status.ObservedGeneration)

	if !appWorkload.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	statefulSet, err := r.workloadsToStSet.Convert(appWorkload)
	if err != nil {
		log.Info("error when converting AppWorkload", "reason", err)
		return ctrl.Result{}, err
	}

	createdStSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSet.Name,
			Namespace: statefulSet.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, createdStSet, func() error {
		createdStSet.Labels = statefulSet.Labels
		createdStSet.Annotations = statefulSet.Annotations
		createdStSet.OwnerReferences = statefulSet.OwnerReferences
		createdStSet.Spec = statefulSet.Spec

		return nil
	})
	if err != nil {
		log.Info("error when creating or updating StatefulSet", "reason", err)
		return ctrl.Result{}, err
	}

	err = r.pdb.Update(ctx, createdStSet)
	if err != nil {
		log.Info("error when creating or patching pod disruption budget", "reason", err)
		return ctrl.Result{}, err
	}

	appWorkload.Status.ActualInstances = createdStSet.Status.ReadyReplicas

	instancesState, err := r.stateCollector.CollectState(ctx, appWorkload.Spec.GUID)
	if err != nil {
		log.Info("error when collecting instances state", "reason", err)
		return ctrl.Result{}, err
	}
	appWorkload.Status.InstancesStatus = instancesState

	return ctrl.Result{}, nil
}
