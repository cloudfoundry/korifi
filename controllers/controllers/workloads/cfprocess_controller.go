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
	"crypto/sha1"
	"errors"
	"fmt"
	"sort"
	"strconv"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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

//counterfeiter:generate -o fake -fake-name EnvBuilder . EnvBuilder
type EnvBuilder interface {
	BuildEnv(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error)
}

// CFProcessReconciler reconciles a CFProcess object
type CFProcessReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Log        logr.Logger
	EnvBuilder EnvBuilder
}

func NewCFProcessReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, envBuilder EnvBuilder) *CFProcessReconciler {
	return &CFProcessReconciler{Client: client, Scheme: scheme, Log: log, EnvBuilder: envBuilder}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses/finalizers,verbs=update
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=runworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

func (r *CFProcessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfProcess := new(korifiv1alpha1.CFProcess)
	var err error
	err = r.Client.Get(ctx, req.NamespacedName, cfProcess)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFProcess %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfProcess.Spec.AppRef.Name, Namespace: cfProcess.Namespace}, cfApp)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFApp %s/%s", req.Namespace, cfProcess.Spec.AppRef.Name))
		return ctrl.Result{}, err
	}

	err = r.setOwnerRef(ctx, cfProcess, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfAppRev := korifiv1alpha1.CFAppRevisionKeyDefault
	if foundValue, ok := cfApp.GetAnnotations()[korifiv1alpha1.CFAppRevisionKey]; ok {
		cfAppRev = foundValue
	}

	if cfApp.Spec.DesiredState == korifiv1alpha1.StartedState {
		err = r.createOrPatchRunWorkload(ctx, cfApp, cfProcess, cfAppRev)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.cleanUpRunWorkloads(ctx, cfProcess, cfApp.Spec.DesiredState, cfAppRev)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFProcessReconciler) createOrPatchRunWorkload(ctx context.Context, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess, cfAppRev string) error {
	cfBuild := new(korifiv1alpha1.CFBuild)
	err := r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfProcess.Namespace}, cfBuild)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFBuild %s/%s", cfProcess.Namespace, cfApp.Spec.CurrentDropletRef.Name))
		return err
	}

	if cfBuild.Status.Droplet == nil {
		r.Log.Error(err, fmt.Sprintf("No build droplet status on CFBuild %s/%s", cfProcess.Namespace, cfApp.Spec.CurrentDropletRef.Name))
		return errors.New("no build droplet status on CFBuild")
	}

	var appPort int
	appPort, err = r.getPort(ctx, cfProcess, cfApp)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch routes for CFApp %s/%s", cfProcess.Namespace, cfApp.Spec.DisplayName))
		return err
	}

	envVars, err := r.EnvBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("error when trying build the process environment for app: %s/%s", cfProcess.Namespace, cfApp.Spec.DisplayName))
		return err
	}

	actualRunWorkload := &korifiv1alpha1.RunWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfProcess.Namespace,
			Name:      generateRunWorkloadName(cfAppRev, cfProcess.Name),
		},
	}

	var desiredRunWorkload *korifiv1alpha1.RunWorkload
	desiredRunWorkload, err = r.generateRunWorkload(actualRunWorkload, cfApp, cfProcess, cfBuild, appPort, envVars)
	if err != nil { // untested
		r.Log.Error(err, "Error when initializing RunWorkload")
		return err
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, actualRunWorkload, runWorkloadMutateFunction(actualRunWorkload, desiredRunWorkload))
	if err != nil {
		r.Log.Error(err, "Error calling CreateOrPatch on RunWorkload")
		return err
	}
	return nil
}

func (r *CFProcessReconciler) setOwnerRef(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess, cfApp *korifiv1alpha1.CFApp) error {
	originalCFProcess := cfProcess.DeepCopy()
	err := controllerutil.SetOwnerReference(cfApp, cfProcess, r.Scheme)
	if err != nil {
		r.Log.Error(err, "unable to set owner reference on CFProcess")
		return err
	}

	err = r.Client.Patch(ctx, cfProcess, client.MergeFrom(originalCFProcess))
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error setting owner reference on the CFProcess %s/%s", cfProcess.Namespace, cfProcess.Name))
		return err
	}
	return nil
}

func (r *CFProcessReconciler) cleanUpRunWorkloads(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess, desiredState korifiv1alpha1.DesiredState, cfAppRev string) error {
	runWorkloadsForProcess, err := r.fetchRunWorkloadsForProcess(ctx, cfProcess)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch RunWorkloads for Process %s/%s", cfProcess.Namespace, cfProcess.Name))
		return err
	}

	for i, currentRunWorkload := range runWorkloadsForProcess {
		if desiredState == korifiv1alpha1.StoppedState || currentRunWorkload.Labels[korifiv1alpha1.CFAppRevisionKey] != cfAppRev {
			err := r.Client.Delete(ctx, &runWorkloadsForProcess[i])
			if err != nil {
				r.Log.Info(fmt.Sprintf("Error occurred deleting RunWorkload: %s, %s", currentRunWorkload.Name, err))
				return err
			}
		}
	}
	return nil
}

func runWorkloadMutateFunction(actualrunWorkload, desiredrunWorkload *korifiv1alpha1.RunWorkload) controllerutil.MutateFn {
	return func() error {
		actualrunWorkload.ObjectMeta.Labels = desiredrunWorkload.ObjectMeta.Labels
		actualrunWorkload.ObjectMeta.Annotations = desiredrunWorkload.ObjectMeta.Annotations
		actualrunWorkload.ObjectMeta.OwnerReferences = desiredrunWorkload.ObjectMeta.OwnerReferences
		actualrunWorkload.Spec = desiredrunWorkload.Spec
		return nil
	}
}

func (r *CFProcessReconciler) generateRunWorkload(actualRunWorkload *korifiv1alpha1.RunWorkload, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess, cfBuild *korifiv1alpha1.CFBuild, appPort int, envVars []corev1.EnvVar) (*korifiv1alpha1.RunWorkload, error) {
	var desiredRunWorkload korifiv1alpha1.RunWorkload
	actualRunWorkload.DeepCopyInto(&desiredRunWorkload)

	desiredRunWorkload.Labels = make(map[string]string)
	desiredRunWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey] = cfApp.Name
	cfAppRevisionKeyValue := korifiv1alpha1.CFAppRevisionKeyDefault
	if cfApp.Annotations != nil {
		if foundValue, has := cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey]; has {
			cfAppRevisionKeyValue = foundValue
		}
	}
	desiredRunWorkload.Labels[korifiv1alpha1.CFAppRevisionKey] = cfAppRevisionKeyValue
	desiredRunWorkload.Labels[korifiv1alpha1.CFProcessGUIDLabelKey] = cfProcess.Name
	desiredRunWorkload.Labels[korifiv1alpha1.CFProcessTypeLabelKey] = cfProcess.Spec.ProcessType

	desiredRunWorkload.Spec.GUID = cfProcess.Name
	desiredRunWorkload.Spec.Version = cfAppRevisionKeyValue
	desiredRunWorkload.Spec.DiskMiB = cfProcess.Spec.DiskQuotaMB
	desiredRunWorkload.Spec.MemoryMiB = cfProcess.Spec.MemoryMB
	desiredRunWorkload.Spec.ProcessType = cfProcess.Spec.ProcessType
	desiredRunWorkload.Spec.Command = commandForProcess(cfProcess, cfApp)
	desiredRunWorkload.Spec.AppGUID = cfApp.Name
	desiredRunWorkload.Spec.Image = cfBuild.Status.Droplet.Registry.Image
	desiredRunWorkload.Spec.ImagePullSecrets = cfBuild.Status.Droplet.Registry.ImagePullSecrets
	desiredRunWorkload.Spec.Ports = cfProcess.Spec.Ports
	desiredRunWorkload.Spec.Instances = int32(cfProcess.Spec.DesiredInstances)

	desiredRunWorkload.Spec.Env = generateEnvVars(appPort, envVars)
	desiredRunWorkload.Spec.Health = korifiv1alpha1.Healthcheck{
		Type:      string(cfProcess.Spec.HealthCheck.Type),
		Port:      int32(appPort),
		Endpoint:  cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
		TimeoutMs: uint(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds * 1000),
	}
	desiredRunWorkload.Spec.CPUWeight = 0

	err := controllerutil.SetOwnerReference(cfProcess, &desiredRunWorkload, r.Scheme)
	if err != nil {
		return nil, err
	}

	return &desiredRunWorkload, err
}

func generateRunWorkloadName(cfAppRev string, processGUID string) string {
	h := sha1.New()
	h.Write([]byte(cfAppRev))
	appRevHash := h.Sum(nil)
	runWorkloadName := processGUID + fmt.Sprintf("-%x", appRevHash)[:5]
	return runWorkloadName
}

func (r *CFProcessReconciler) fetchRunWorkloadsForProcess(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess) ([]korifiv1alpha1.RunWorkload, error) {
	allRunWorkloads := &korifiv1alpha1.RunWorkloadList{}
	err := r.Client.List(ctx, allRunWorkloads, client.InNamespace(cfProcess.Namespace))
	if err != nil {
		return []korifiv1alpha1.RunWorkload{}, err
	}
	var runWorkloadsForProcess []korifiv1alpha1.RunWorkload
	for _, currentRunWorkload := range allRunWorkloads.Items {
		if processGUID, has := currentRunWorkload.Labels[korifiv1alpha1.CFProcessGUIDLabelKey]; has && processGUID == cfProcess.Name {
			runWorkloadsForProcess = append(runWorkloadsForProcess, currentRunWorkload)
		}
	}
	return runWorkloadsForProcess, err
}

func (r *CFProcessReconciler) getPort(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess, cfApp *korifiv1alpha1.CFApp) (int, error) {
	// Get Routes for the process
	var cfRoutesForProcess korifiv1alpha1.CFRouteList
	err := r.Client.List(ctx, &cfRoutesForProcess, client.InNamespace(cfApp.GetNamespace()), client.MatchingFields{shared.IndexRouteDestinationAppName: cfApp.Name})
	if err != nil {
		return 0, err
	}

	// In case there are multiple routes, prefer the oldest one
	sort.Slice(cfRoutesForProcess.Items, func(i, j int) bool {
		return cfRoutesForProcess.Items[i].CreationTimestamp.Before(&cfRoutesForProcess.Items[j].CreationTimestamp)
	})

	// Filter those destinations
	for _, cfRoute := range cfRoutesForProcess.Items {
		for _, destination := range cfRoute.Status.Destinations {
			if destination.AppRef.Name == cfApp.Name && destination.ProcessType == cfProcess.Spec.ProcessType && destination.Port != 0 {
				// Just use the first candidate port
				return destination.Port, nil
			}
		}
	}

	return 8080, nil
}

func generateEnvVars(port int, commonEnv []corev1.EnvVar) []corev1.EnvVar {
	var result []corev1.EnvVar
	result = append(result, commonEnv...)
	portString := strconv.Itoa(port)
	result = append(result,
		corev1.EnvVar{Name: "VCAP_APP_HOST", Value: "0.0.0.0"},
		corev1.EnvVar{Name: "VCAP_APP_PORT", Value: portString},
		corev1.EnvVar{Name: "PORT", Value: portString},
	)

	return result
}

func commandForProcess(process *korifiv1alpha1.CFProcess, app *korifiv1alpha1.CFApp) []string {
	if process.Spec.Command == "" {
		return []string{}
	} else if app.Spec.Lifecycle.Type == korifiv1alpha1.BuildpackLifecycle {
		return []string{"/cnb/lifecycle/launcher", process.Spec.Command}
	} else {
		return []string{"/bin/sh", "-c", process.Spec.Command}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFProcessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFProcess{}).
		Watches(&source.Kind{Type: &korifiv1alpha1.CFApp{}}, handler.EnqueueRequestsFromMapFunc(func(app client.Object) []reconcile.Request {
			processList := &korifiv1alpha1.CFProcessList{}
			err := mgr.GetClient().List(context.Background(), processList, client.InNamespace(app.GetNamespace()), client.MatchingLabels{korifiv1alpha1.CFAppGUIDLabelKey: app.GetName()})
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("Error when trying to list CFProcesses in namespace %q", app.GetNamespace()))
				return []reconcile.Request{}
			}

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
