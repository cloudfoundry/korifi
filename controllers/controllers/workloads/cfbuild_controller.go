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
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

//counterfeiter:generate -o fake -fake-name BuildCleaner . BuildCleaner

type BuildCleaner interface {
	Clean(ctx context.Context, app types.NamespacedName) error
}

// CFBuildReconciler reconciles a CFBuild object
type CFBuildReconciler struct {
	k8sClient        client.Client
	buildCleaner     BuildCleaner
	scheme           *runtime.Scheme
	log              logr.Logger
	controllerConfig *config.ControllerConfig
	envBuilder       EnvBuilder
}

func NewCFBuildReconciler(
	k8sClient client.Client,
	buildCleaner BuildCleaner,
	scheme *runtime.Scheme,
	log logr.Logger,
	controllerConfig *config.ControllerConfig,
	envBuilder EnvBuilder,
) *k8s.PatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild] {
	buildReconciler := CFBuildReconciler{
		k8sClient:        k8sClient,
		buildCleaner:     buildCleaner,
		scheme:           scheme,
		log:              log,
		controllerConfig: controllerConfig,
		envBuilder:       envBuilder,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild](log, k8sClient, &buildReconciler)
}

func (r *CFBuildReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFBuild{}).
		Watches(
			&source.Kind{Type: &korifiv1alpha1.BuildWorkload{}},
			handler.EnqueueRequestsFromMapFunc(buildworkloadToBuild),
		)
}

func buildworkloadToBuild(o client.Object) []reconcile.Request {
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

func (r *CFBuildReconciler) ReconcileResource(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", cfBuild.Namespace, "name", cfBuild.Name)

	cfBuild.Status.ObservedGeneration = cfBuild.Generation
	log.V(1).Info("set observed generation", "generation", cfBuild.Status.ObservedGeneration)

	cfApp := new(korifiv1alpha1.CFApp)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.AppRef.Name, Namespace: cfBuild.Namespace}, cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	cfPackage := new(korifiv1alpha1.CFPackage)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.PackageRef.Name, Namespace: cfBuild.Namespace}, cfPackage)
	if err != nil {
		log.Info("error when fetching CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetOwnerReference(cfApp, cfBuild, r.scheme)
	if err != nil {
		log.Info("unable to set owner reference on CFBuild", "reason", err)
		return ctrl.Result{}, err
	}

	err = r.buildCleaner.Clean(ctx, types.NamespacedName{Name: cfApp.Name, Namespace: cfBuild.Namespace})
	if err != nil {
		log.Info("unable to clean old builds", "reason", err)
	}

	stagingStatus := shared.GetConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType, cfBuild.Generation)
	succeededStatus := shared.GetConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType, cfBuild.Generation)

	if succeededStatus != metav1.ConditionUnknown {
		return ctrl.Result{}, nil
	}

	if stagingStatus == metav1.ConditionUnknown {
		err = r.createBuildWorkload(ctx, cfBuild, cfApp, cfPackage)
		if err != nil {
			log.Info("failed to create BuildWorkload", "reason", err)
		}

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.StagingConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "BuildRunning",
			ObservedGeneration: cfBuild.Generation,
		})

		return ctrl.Result{}, err
	}

	var buildWorkload korifiv1alpha1.BuildWorkload
	err = r.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), &buildWorkload)
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

func (r *CFBuildReconciler) createBuildWorkload(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild, cfApp *korifiv1alpha1.CFApp, cfPackage *korifiv1alpha1.CFPackage) error {
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
		return fmt.Errorf("prepareBuildServices: %w", err)
	}
	desiredWorkload.Spec.Services = buildServices

	imageEnvironment, err := r.envBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		return fmt.Errorf("prepareEnvironment: %w", err)
	}
	desiredWorkload.Spec.Env = imageEnvironment

	err = controllerutil.SetControllerReference(cfBuild, &desiredWorkload, r.scheme)
	if err != nil {
		return fmt.Errorf("failed to set OwnerRef on BuildWorkload: %w", err)
	}

	err = r.createBuildWorkloadIfNotExists(ctx, desiredWorkload)
	if err != nil {
		return fmt.Errorf("createBuildWorkloadIfNotExists: %w", err)
	}

	return nil
}

func (r *CFBuildReconciler) prepareBuildServices(ctx context.Context, namespace, appGUID string) ([]corev1.ObjectReference, error) {
	serviceBindingsList := &korifiv1alpha1.CFServiceBindingList{}
	err := r.k8sClient.List(ctx, serviceBindingsList,
		client.InNamespace(namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: appGUID},
	)
	if err != nil {
		return nil, fmt.Errorf("error listing CFServiceBindings: %w", err)
	}

	var buildServices []corev1.ObjectReference
	for _, serviceBinding := range serviceBindingsList.Items {
		if serviceBinding.Status.Binding.Name != "" {
			objRef := corev1.ObjectReference{
				Kind:       "Secret",
				Name:       serviceBinding.Status.Binding.Name,
				APIVersion: "v1",
			}
			buildServices = append(buildServices, objRef)
		} else {
			return nil, errors.New("binding secret name is empty")
		}
	}
	return buildServices, nil
}

func (r *CFBuildReconciler) createBuildWorkloadIfNotExists(ctx context.Context, desiredWorkload korifiv1alpha1.BuildWorkload) error {
	var foundWorkload korifiv1alpha1.BuildWorkload
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(&desiredWorkload), &foundWorkload)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.k8sClient.Create(ctx, &desiredWorkload)
			if err != nil {
				return fmt.Errorf("error creating BuildWorkload: %w", err)
			}
		} else {
			return fmt.Errorf("error checking if BuildWorkload exists: %w", err)
		}
	}
	return nil
}
