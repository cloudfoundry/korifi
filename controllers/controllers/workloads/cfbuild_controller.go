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

// CFBuildReconciler reconciles a CFBuild object
type CFBuildReconciler struct {
	k8sClient        client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	controllerConfig *config.ControllerConfig
	envBuilder       EnvBuilder
}

func NewCFBuildReconciler(k8sClient client.Client, scheme *runtime.Scheme, log logr.Logger, controllerConfig *config.ControllerConfig, envBuilder EnvBuilder) *k8s.PatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild] {
	buildReconciler := CFBuildReconciler{k8sClient: k8sClient, scheme: scheme, log: log, controllerConfig: controllerConfig, envBuilder: envBuilder}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFBuild, *korifiv1alpha1.CFBuild](log, k8sClient, &buildReconciler)
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfpackages/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get

func (r *CFBuildReconciler) ReconcileResource(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild) (ctrl.Result, error) {
	cfApp := new(korifiv1alpha1.CFApp)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.AppRef.Name, Namespace: cfBuild.Namespace}, cfApp)
	if err != nil {
		r.log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	cfPackage := new(korifiv1alpha1.CFPackage)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.PackageRef.Name, Namespace: cfBuild.Namespace}, cfPackage)
	if err != nil {
		r.log.Error(err, "Error when fetching CFPackage")
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(cfPackage, cfBuild, r.scheme)
	if err != nil {
		r.log.Error(err, "unable to set owner reference on CFBuild")
		return ctrl.Result{}, err
	}

	stagingStatus := getConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)
	succeededStatus := getConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)

	if succeededStatus != metav1.ConditionUnknown {
		return ctrl.Result{}, nil
	}

	if stagingStatus == metav1.ConditionUnknown {
		err = r.createBuildWorkload(ctx, cfBuild, cfApp, cfPackage)
		return ctrl.Result{}, err
	}

	var buildWorkload korifiv1alpha1.BuildWorkload
	err = r.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), &buildWorkload)
	if err != nil {
		r.log.Error(err, "Error when fetching BuildWorkload")
		// Ignore NotFound errors to account for eventual consistency
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	workloadSucceededStatus := meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)
	if workloadSucceededStatus == nil {
		return ctrl.Result{}, nil
	}

	switch workloadSucceededStatus.Status {
	case metav1.ConditionFalse:
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.StagingConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuildWorkload",
			Message: "BuildWorkload",
		})

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuildWorkload",
			Message: fmt.Sprintf("%s: %s", workloadSucceededStatus.Reason, workloadSucceededStatus.Message),
		})
	case metav1.ConditionTrue:
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.StagingConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuildWorkload",
			Message: "BuildWorkload",
		})

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "BuildWorkload",
			Message: "BuildWorkload",
		})

		cfBuild.Status.Droplet = buildWorkload.Status.Droplet
	default:
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *CFBuildReconciler) createBuildWorkload(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild, cfApp *korifiv1alpha1.CFApp, cfPackage *korifiv1alpha1.CFPackage) error {
	namespace := cfBuild.Namespace

	buildServices, err := r.prepareBuildServices(ctx, namespace, cfApp.Name)
	if err != nil {
		return fmt.Errorf("prepareBuildServices: %w", err)
	}

	imageEnvironment, err := r.envBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		r.log.Error(err, "failed building environment")
		return fmt.Errorf("prepareEnvironment: %w", err)
	}

	desiredWorkload := &korifiv1alpha1.BuildWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfBuild.Name,
			Namespace: namespace,
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, desiredWorkload, func() error {
		if desiredWorkload.Labels == nil {
			desiredWorkload.Labels = map[string]string{}
		}

		desiredWorkload.Labels[korifiv1alpha1.CFBuildGUIDLabelKey] = cfBuild.Name
		desiredWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey] = cfApp.Name

		desiredWorkload.Spec = korifiv1alpha1.BuildWorkloadSpec{
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
			Services:    buildServices,
			Env:         imageEnvironment,
		}

		return controllerutil.SetControllerReference(cfBuild, desiredWorkload, r.scheme)
	})

	if err != nil {
		return fmt.Errorf("failed to createOrPatch build workload: %w", err)
	}

	meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
		Type:    korifiv1alpha1.StagingConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "BuildWorkload",
		Message: "BuildWorkload",
	})

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
			r.log.Info("binding secret name is empty")
			return nil, errors.New("binding secret name is empty")
		}
	}
	return buildServices, nil
}

// getConditionOrSetAsUnknown is a helper function that retrieves the value of the provided conditionType, like
// "Succeeded" and returns the value: "True", "False", or "Unknown". If the value is not present, the pointer to the
// list of conditions provided to the function is used to add an entry to the list of Conditions with a value of
// "Unknown" and "Unknown" is returned
func getConditionOrSetAsUnknown(conditions *[]metav1.Condition, conditionType string) metav1.ConditionStatus {
	if conditionStatus := meta.FindStatusCondition(*conditions, conditionType); conditionStatus != nil {
		return conditionStatus.Status
	}

	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "Unknown",
		Message: "Unknown",
	})
	return metav1.ConditionUnknown
}

func (r *CFBuildReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
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
			}))
}
