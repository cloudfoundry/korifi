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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// CFBuildReconciler reconciles a CFBuild object
type CFBuildReconciler struct {
	Client           CFClient
	Scheme           *runtime.Scheme
	Log              logr.Logger
	ControllerConfig *config.ControllerConfig
	EnvBuilder       EnvBuilder
}

func NewCFBuildReconciler(client CFClient, scheme *runtime.Scheme, log logr.Logger, controllerConfig *config.ControllerConfig, envBuilder EnvBuilder) *CFBuildReconciler {
	return &CFBuildReconciler{Client: client, Scheme: scheme, Log: log, ControllerConfig: controllerConfig, EnvBuilder: envBuilder}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get

func (r *CFBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfBuild := new(korifiv1alpha1.CFBuild)
	err := r.Client.Get(ctx, req.NamespacedName, cfBuild)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "Error when fetching CFBuild")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.AppRef.Name, Namespace: cfBuild.Namespace}, cfApp)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	originalCFBuild := cfBuild.DeepCopy()
	err = controllerutil.SetOwnerReference(cfApp, cfBuild, r.Scheme)
	if err != nil {
		r.Log.Error(err, "unable to set owner reference on CFBuild")
		return ctrl.Result{}, err
	}

	err = r.Client.Patch(ctx, cfBuild, client.MergeFrom(originalCFBuild))
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error setting owner reference on the CFBuild %s/%s", req.Namespace, cfBuild.Name))
		return ctrl.Result{}, err
	}

	if isBuildCompleted(cfBuild) {
		return ctrl.Result{}, nil
	}

	if !isBuildStaged(cfBuild) {
		cfPackage := new(korifiv1alpha1.CFPackage)
		err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.PackageRef.Name, Namespace: cfBuild.Namespace}, cfPackage)
		if err != nil {
			r.Log.Error(err, "Error when fetching CFPackage")
			return ctrl.Result{}, err
		}

		err = r.createBuildWorkloadAndPatchStatus(ctx, cfBuild, cfApp, cfPackage)
		return ctrl.Result{}, err
	}

	var buildWorkload korifiv1alpha1.BuildWorkload
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(cfBuild), &buildWorkload)
	if err != nil {
		r.Log.Error(err, "Error when fetching BuildWorkload")
		// Ignore NotFound errors to account for eventual consistency
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	workloadSucceededStatus := meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)
	if workloadSucceededStatus == nil {
		return ctrl.Result{}, nil
	}

	switch workloadSucceededStatus.Status {
	case metav1.ConditionFalse:
		return ctrl.Result{}, k8s.PatchStatusConditions(ctx, r.Client, cfBuild,
			metav1.Condition{
				Type:    korifiv1alpha1.StagingConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "BuildWorkload",
				Message: "BuildWorkload",
			},
			metav1.Condition{
				Type:    korifiv1alpha1.SucceededConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "BuildWorkload",
				Message: fmt.Sprintf("%s:%s", workloadSucceededStatus.Reason, workloadSucceededStatus.Message),
			},
		)
	case metav1.ConditionTrue:
		return ctrl.Result{}, k8s.PatchStatus(ctx, r.Client, cfBuild, func() {
			cfBuild.Status.Droplet = buildWorkload.Status.Droplet
		},
			metav1.Condition{
				Type:    korifiv1alpha1.StagingConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "BuildWorkload",
				Message: "BuildWorkload",
			},
			metav1.Condition{
				Type:    korifiv1alpha1.SucceededConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "BuildWorkload",
				Message: "BuildWorkload",
			},
		)
	default:
		return ctrl.Result{}, nil
	}
}

func isBuildCompleted(cfBuild *korifiv1alpha1.CFBuild) bool {
	return meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType) != nil
}

func isBuildStaged(cfBuild *korifiv1alpha1.CFBuild) bool {
	return meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType) != nil
}

func (r *CFBuildReconciler) createBuildWorkloadAndPatchStatus(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild, cfApp *korifiv1alpha1.CFApp, cfPackage *korifiv1alpha1.CFPackage) error {
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
			ReconcilerName: r.ControllerConfig.BuildReconciler,
		},
	}

	buildServices, err := r.prepareBuildServices(ctx, namespace, cfApp.Name)
	if err != nil {
		return fmt.Errorf("prepareBuildServices: %w", err)
	}
	desiredWorkload.Spec.Services = buildServices

	imageEnvironment, err := r.EnvBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		r.Log.Error(err, "failed building environment")
		return fmt.Errorf("prepareEnvironment: %w", err)
	}
	desiredWorkload.Spec.Env = imageEnvironment

	err = controllerutil.SetOwnerReference(cfBuild, &desiredWorkload, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on BuildWorkload")
		return fmt.Errorf("failed to set OwnerRef on BuildWorkload: %w", err)
	}

	err = r.createBuildWorkloadIfNotExists(ctx, desiredWorkload)
	if err != nil {
		return fmt.Errorf("createBuildWorkloadIfNotExists: %w", err)
	}

	err = k8s.PatchStatusConditions(ctx, r.Client, cfBuild, metav1.Condition{
		Type:    korifiv1alpha1.StagingConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "BuildWorkload",
		Message: "BuildWorkload",
	})
	if err != nil {
		return fmt.Errorf("patch cfbuild status: %w", err)
	}

	return nil
}

func (r *CFBuildReconciler) prepareBuildServices(ctx context.Context, namespace, appGUID string) ([]corev1.ObjectReference, error) {
	serviceBindingsList := &korifiv1alpha1.CFServiceBindingList{}
	err := r.Client.List(ctx, serviceBindingsList,
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
			r.Log.Info("binding secret name is empty")
			return nil, errors.New("binding secret name is empty")
		}
	}
	return buildServices, nil
}

func (r *CFBuildReconciler) createBuildWorkloadIfNotExists(ctx context.Context, desiredWorkload korifiv1alpha1.BuildWorkload) error {
	var foundWorkload korifiv1alpha1.BuildWorkload
	err := r.Client.Get(ctx, client.ObjectKeyFromObject(&desiredWorkload), &foundWorkload)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.Client.Create(ctx, &desiredWorkload)
			if err != nil {
				r.Log.Error(err, "Error when creating BuildWorkload")
				return err
			}
		} else {
			r.Log.Error(err, "Error when checking if BuildWorkload exists")
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFBuild{}).
		Watches(
			&source.Kind{Type: &korifiv1alpha1.BuildWorkload{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.GetLabels()[korifiv1alpha1.CFBuildGUIDLabelKey],
						Namespace: obj.GetNamespace(),
					},
				})
				return requests
			})).
		Complete(r)
}
