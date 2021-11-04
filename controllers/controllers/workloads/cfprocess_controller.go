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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
)

// CFProcessReconciler reconciles a CFProcess object
type CFProcessReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses/finalizers,verbs=update
//+kubebuilder:rbac:groups="eirini.cloudfoundry.org",resources=lrps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *CFProcessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	cfProcess := workloadsv1alpha1.CFProcess{}
	var err error
	err = r.Client.Get(ctx, req.NamespacedName, &cfProcess)

	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFProcess %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := workloadsv1alpha1.CFApp{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfProcess.Spec.AppRef.Name, Namespace: cfProcess.Namespace}, &cfApp)

	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFApp %s/%s", req.Namespace, cfProcess.Spec.AppRef.Name))
		return ctrl.Result{}, err
	}

	lrp := eiriniv1.LRP{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfProcess.Namespace,
			Name:      cfProcess.Name,
		},
	}

	switch cfApp.Spec.DesiredState {
	case workloadsv1alpha1.StartedState:
		cfBuild := workloadsv1alpha1.CFBuild{}
		err = r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfProcess.Namespace}, &cfBuild)

		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFBuild %s/%s", req.Namespace, cfApp.Spec.CurrentDropletRef.Name))
			return ctrl.Result{}, err
		}

		if cfBuild.Status.BuildDropletStatus == nil {
			r.Log.Error(err, fmt.Sprintf("No build droplet status on CFBuild %s/%s", req.Namespace, cfApp.Spec.CurrentDropletRef.Name))
			return ctrl.Result{}, errors.New("no build droplet status on CFBuild")
		}

		appEnvSecret := corev1.Secret{}
		if cfApp.Spec.EnvSecretName != "" {
			err = r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.EnvSecretName, Namespace: cfProcess.Namespace}, &appEnvSecret)
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("Error when trying to fetch app env Secret %s/%s", req.Namespace, cfApp.Spec.EnvSecretName))
				return ctrl.Result{}, err
			}
		}

		_, err = controllerutil.CreateOrPatch(ctx, r.Client, &lrp, func() error {
			lrp.Spec.GUID = cfProcess.Name
			lrp.Spec.DiskMB = cfProcess.Spec.DiskQuotaMB
			lrp.Spec.MemoryMB = cfProcess.Spec.MemoryMB
			lrp.Spec.ProcessType = cfProcess.Spec.ProcessType
			lrp.Spec.Command = commandForProcess(&cfProcess, &cfApp)
			lrp.Spec.AppName = cfApp.Spec.Name
			lrp.Spec.AppGUID = cfApp.Name
			lrp.Spec.Image = cfBuild.Status.BuildDropletStatus.Registry.Image
			lrp.Spec.Ports = cfProcess.Spec.Ports
			lrp.Spec.Instances = cfProcess.Spec.DesiredInstances
			lrp.Spec.Env = secretDataToEnvMap(appEnvSecret.Data)
			lrp.Spec.Health = eiriniv1.Healthcheck{
				Type:      string(cfProcess.Spec.HealthCheck.Type),
				Port:      cfProcess.Spec.Ports[0], // Is this always the first process port?
				Endpoint:  cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
				TimeoutMs: uint(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds * 1000),
			}
			lrp.Spec.CPUWeight = 0
			lrp.Spec.Sidecars = nil

			err = controllerutil.SetOwnerReference(&cfProcess, &lrp, r.Scheme)
			if err != nil {
				r.Log.Error(err, "failed to set OwnerRef on LRP")
				return err
			}

			return nil
		})
		if err != nil {
			r.Log.Error(err, "Error calling CreateOrPatch on lrp")
			return ctrl.Result{}, err
		}
	case workloadsv1alpha1.StoppedState:
		err = r.Client.Delete(ctx, &lrp)
		if err != nil {
			r.Log.Info(fmt.Sprintf("Error occurred deleting LRP: %s, %s", cfProcess.Name, err))
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	return ctrl.Result{}, nil
}

func secretDataToEnvMap(secretData map[string][]byte) map[string]string {
	convertedMap := make(map[string]string)
	for k, v := range secretData {
		convertedMap[k] = string(v)
	}
	return convertedMap
}

func commandForProcess(process *workloadsv1alpha1.CFProcess, app *workloadsv1alpha1.CFApp) []string {
	if process.Spec.Command == "" {
		return []string{}
	} else if app.Spec.Lifecycle.Type == workloadsv1alpha1.BuildpackLifecycle {
		return []string{"/cnb/lifecycle/launcher", process.Spec.Command}
	} else {
		return []string{"/bin/sh", "-c", process.Spec.Command}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFProcessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFProcess{}).
		Watches(&source.Kind{Type: &workloadsv1alpha1.CFApp{}}, handler.EnqueueRequestsFromMapFunc(func(app client.Object) []reconcile.Request {
			processList := &workloadsv1alpha1.CFProcessList{}
			_ = mgr.GetClient().List(context.Background(), processList, client.InNamespace(app.GetNamespace()), client.MatchingLabels{workloadsv1alpha1.CFAppGUIDLabelKey: app.GetName()})
			var requests []reconcile.Request

			for _, process := range processList.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      process.Name,
						Namespace: process.Namespace,
					},
				})
			}
			return requests
		})).
		Complete(r)
}
