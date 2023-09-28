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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

func NewCFBuildpackBuildReconciler(
	k8sClient client.Client,
	buildCleaner build.BuildCleaner,
	scheme *runtime.Scheme,
	log logr.Logger,
	controllerConfig *config.ControllerConfig,
	envBuilder EnvBuilder,
) *k8s.PatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild](
		log,
		k8sClient,
		build.NewCFBuildReconciler(
			log,
			k8sClient,
			scheme,
			buildCleaner,
			&buildpackBuildReconciler{
				k8sClient:        k8sClient,
				controllerConfig: controllerConfig,
				envBuilder:       envBuilder,
				scheme:           scheme,
			},
		))
}

type buildpackBuildReconciler struct {
	k8sClient        client.Client
	controllerConfig *config.ControllerConfig
	envBuilder       EnvBuilder
	scheme           *runtime.Scheme
}

func (r *buildpackBuildReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFBuild{}).
		Watches(
			&korifiv1alpha1.BuildWorkload{},
			handler.EnqueueRequestsFromMapFunc(buildworkloadToBuild),
		).
		WithEventFilter(predicate.NewPredicateFuncs(buildpackBuildFilter))
}

func buildpackBuildFilter(object client.Object) bool {
	_, ok := object.(*korifiv1alpha1.BuildWorkload)
	if ok {
		return true
	}

	cfBuild, ok := object.(*korifiv1alpha1.CFBuild)
	if !ok {
		return false
	}

	return cfBuild.Spec.Lifecycle.Type == korifiv1alpha1.LifecycleType("buildpack")
}

func buildworkloadToBuild(ctx context.Context, o client.Object) []reconcile.Request {
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      o.GetLabels()[korifiv1alpha1.CFBuildGUIDLabelKey],
				Namespace: o.GetNamespace(),
			},
		},
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/finalizers,verbs=update

func (r *buildpackBuildReconciler) ReconcileBuild(
	ctx context.Context,
	cfBuild *korifiv1alpha1.CFBuild,
	cfApp *korifiv1alpha1.CFApp,
	cfPackage *korifiv1alpha1.CFPackage,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	stagingStatus := shared.GetConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType, cfBuild.Generation)
	if stagingStatus == metav1.ConditionUnknown {
		err := r.createBuildWorkload(ctx, cfBuild, cfApp, cfPackage)
		if err != nil {
			log.Info("failed to create BuildWorkload", "reason", err)
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.StagingConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "BuildRunning",
			ObservedGeneration: cfBuild.Generation,
		})

		return ctrl.Result{}, nil
	}

	var buildWorkload korifiv1alpha1.BuildWorkload
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), &buildWorkload)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		} else {
			log.Info("error when fetching BuildWorkload", "reason", err)
			return ctrl.Result{}, err
		}
	}

	workloadSucceededStatus := meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)
	if workloadSucceededStatus == nil {
		return ctrl.Result{}, nil
	}

	switch workloadSucceededStatus.Status {
	case metav1.ConditionFalse:
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.StagingConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuildNotRunning",
			ObservedGeneration: cfBuild.Generation,
		})

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuildFailed",
			Message:            fmt.Sprintf("%s: %s", workloadSucceededStatus.Reason, workloadSucceededStatus.Message),
			ObservedGeneration: cfBuild.Generation,
		})
	case metav1.ConditionTrue:
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.StagingConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuildNotRunning",
			ObservedGeneration: cfBuild.Generation,
		})

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "BuildSucceeded",
			ObservedGeneration: cfBuild.Generation,
		})

		cfBuild.Status.Droplet = buildWorkload.Status.Droplet
	default:
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *buildpackBuildReconciler) createBuildWorkload(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild, cfApp *korifiv1alpha1.CFApp, cfPackage *korifiv1alpha1.CFPackage) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createBuildWorkload")

	namespace := cfBuild.Namespace
	desiredWorkload := korifiv1alpha1.BuildWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfBuild.Name,
			Namespace: namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFBuildGUIDLabelKey: cfBuild.Name,
				korifiv1alpha1.CFAppGUIDLabelKey:   cfApp.Name,
			},
		},
		Spec: korifiv1alpha1.BuildWorkloadSpec{
			BuildRef: korifiv1alpha1.RequiredLocalObjectReference{
				Name: cfBuild.Name,
			},
			Source: korifiv1alpha1.PackageSource{
				Registry: korifiv1alpha1.Registry{
					Image:            cfPackage.Spec.Source.Registry.Image,
					ImagePullSecrets: cfPackage.Spec.Source.Registry.ImagePullSecrets,
				},
			},
			BuilderName: r.controllerConfig.BuilderName,
			Buildpacks:  cfBuild.Spec.Lifecycle.Data.Buildpacks,
		},
	}

	buildServices, err := r.prepareBuildServices(ctx, namespace, cfApp.Name)
	if err != nil {
		return err
	}
	desiredWorkload.Spec.Services = buildServices

	imageEnvironment, err := r.envBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		log.Info("failed to build environment", "reason", err)
		return err
	}
	desiredWorkload.Spec.Env = imageEnvironment

	err = controllerutil.SetControllerReference(cfBuild, &desiredWorkload, r.scheme)
	if err != nil {
		log.Info("failed to set OwnerRef on BuildWorkload", "reason", err)
		return err
	}

	err = r.createBuildWorkloadIfNotExists(ctx, desiredWorkload)
	if err != nil {
		return err
	}

	return nil
}

func (r *buildpackBuildReconciler) prepareBuildServices(ctx context.Context, namespace, appGUID string) ([]corev1.ObjectReference, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("prepareBuildServices")

	serviceBindingsList := &korifiv1alpha1.CFServiceBindingList{}
	err := r.k8sClient.List(ctx, serviceBindingsList,
		client.InNamespace(namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: appGUID},
	)
	if err != nil {
		log.Info("error listing CFServiceBindings", "reason", err)
		return nil, err
	}

	var buildServices []corev1.ObjectReference
	for _, serviceBinding := range serviceBindingsList.Items {
		if serviceBinding.Status.Binding.Name == "" {
			log.Info("binding secret name is empty")
			return nil, err
		}

		objRef := corev1.ObjectReference{
			Kind:       "Secret",
			Name:       serviceBinding.Status.Binding.Name,
			APIVersion: "v1",
		}

		buildServices = append(buildServices, objRef)
	}

	return buildServices, nil
}

func (r *buildpackBuildReconciler) createBuildWorkloadIfNotExists(ctx context.Context, desiredWorkload korifiv1alpha1.BuildWorkload) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createBuildWorkloadIfNotExists")

	var foundWorkload korifiv1alpha1.BuildWorkload
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(&desiredWorkload), &foundWorkload)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.k8sClient.Create(ctx, &desiredWorkload)
			if err != nil {
				log.Info("error creating BuildWorkload", "reason", err)
				return err
			}
		} else {
			log.Info("error checking if BuildWorkload exists", "reason", err)
			return err
		}
	}
	return nil
}
