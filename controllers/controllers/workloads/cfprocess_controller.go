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

	networkingv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	"github.com/go-logr/logr"
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
	BuildEnv(ctx context.Context, cfApp *workloadsv1alpha1.CFApp) (map[string]string, error)
}

// CFProcessReconciler reconciles a CFProcess object
type CFProcessReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Log        logr.Logger
	EnvBuilder EnvBuilder
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses/finalizers,verbs=update
//+kubebuilder:rbac:groups="eirini.cloudfoundry.org",resources=lrps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

func (r *CFProcessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfProcess := new(workloadsv1alpha1.CFProcess)
	var err error
	err = r.Client.Get(ctx, req.NamespacedName, cfProcess)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFProcess %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := new(workloadsv1alpha1.CFApp)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfProcess.Spec.AppRef.Name, Namespace: cfProcess.Namespace}, cfApp)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFApp %s/%s", req.Namespace, cfProcess.Spec.AppRef.Name))
		return ctrl.Result{}, err
	}

	err = r.setOwnerRef(ctx, cfProcess, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfAppRev := workloadsv1alpha1.CFAppRevisionKeyDefault
	if foundValue, ok := cfApp.GetAnnotations()[workloadsv1alpha1.CFAppRevisionKey]; ok {
		cfAppRev = foundValue
	}

	if cfApp.Spec.DesiredState == workloadsv1alpha1.StartedState {
		err = r.createOrPatchLRP(ctx, cfApp, cfProcess, cfAppRev)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.cleanUpLRPs(ctx, cfProcess, cfApp.Spec.DesiredState, cfAppRev)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFProcessReconciler) createOrPatchLRP(ctx context.Context, cfApp *workloadsv1alpha1.CFApp, cfProcess *workloadsv1alpha1.CFProcess, cfAppRev string) error {
	cfBuild := new(workloadsv1alpha1.CFBuild)
	err := r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfProcess.Namespace}, cfBuild)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFBuild %s/%s", cfProcess.Namespace, cfApp.Spec.CurrentDropletRef.Name))
		return err
	}

	if cfBuild.Status.BuildDropletStatus == nil {
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
		r.Log.Error(err, "error when trying build the process environment for app: %s/%s", cfProcess.Namespace, cfApp.Spec.DisplayName)
		return err
	}

	actualLRP := &eiriniv1.LRP{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfProcess.Namespace,
			Name:      generateLRPName(cfAppRev, cfProcess.Name),
		},
	}

	var desiredLRP *eiriniv1.LRP
	desiredLRP, err = r.generateLRP(actualLRP, cfApp, cfProcess, cfBuild, appPort, envVars)
	if err != nil {
		// untested
		r.Log.Error(err, "Error when initializing LRP")
		return err
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, actualLRP, lrpMutateFunction(actualLRP, desiredLRP))
	if err != nil {
		r.Log.Error(err, "Error calling CreateOrPatch on LRP")
		return err
	}
	return nil
}

func (r *CFProcessReconciler) setOwnerRef(ctx context.Context, cfProcess *workloadsv1alpha1.CFProcess, cfApp *workloadsv1alpha1.CFApp) error {
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

func (r *CFProcessReconciler) cleanUpLRPs(ctx context.Context, cfProcess *workloadsv1alpha1.CFProcess, desiredState workloadsv1alpha1.DesiredState, cfAppRev string) error {
	lrpsForProcess, err := r.fetchLRPsForProcess(ctx, cfProcess)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch LRPs for Process %s/%s", cfProcess.Namespace, cfProcess.Name))
		return err
	}

	for _, currentLRP := range lrpsForProcess {
		if desiredState == workloadsv1alpha1.StoppedState || currentLRP.Labels[workloadsv1alpha1.CFAppRevisionKey] != cfAppRev {
			err := r.Client.Delete(ctx, &currentLRP)
			if err != nil {
				r.Log.Info(fmt.Sprintf("Error occurred deleting LRP: %s, %s", currentLRP.Name, err))
				return err
			}
		}
	}
	return nil
}

func lrpMutateFunction(actuallrp, desiredlrp *eiriniv1.LRP) controllerutil.MutateFn {
	return func() error {
		actuallrp.ObjectMeta.Labels = desiredlrp.ObjectMeta.Labels
		actuallrp.ObjectMeta.Annotations = desiredlrp.ObjectMeta.Annotations
		actuallrp.ObjectMeta.OwnerReferences = desiredlrp.ObjectMeta.OwnerReferences
		actuallrp.Spec = desiredlrp.Spec
		return nil
	}
}

func (r *CFProcessReconciler) generateLRP(actualLRP *eiriniv1.LRP, cfApp *workloadsv1alpha1.CFApp, cfProcess *workloadsv1alpha1.CFProcess, cfBuild *workloadsv1alpha1.CFBuild, appPort int, envVars map[string]string) (*eiriniv1.LRP, error) {
	var desiredLRP eiriniv1.LRP
	actualLRP.DeepCopyInto(&desiredLRP)

	desiredLRP.Labels = make(map[string]string)
	desiredLRP.Labels[workloadsv1alpha1.CFAppGUIDLabelKey] = cfApp.Name
	cfAppRevisionKeyValue := workloadsv1alpha1.CFAppRevisionKeyDefault
	if cfApp.Annotations != nil {
		if foundValue, has := cfApp.Annotations[workloadsv1alpha1.CFAppRevisionKey]; has {
			cfAppRevisionKeyValue = foundValue
		}
	}
	desiredLRP.Labels[workloadsv1alpha1.CFAppRevisionKey] = cfAppRevisionKeyValue
	desiredLRP.Labels[workloadsv1alpha1.CFProcessGUIDLabelKey] = cfProcess.Name
	desiredLRP.Labels[workloadsv1alpha1.CFProcessTypeLabelKey] = cfProcess.Spec.ProcessType

	desiredLRP.Spec.GUID = cfProcess.Name
	desiredLRP.Spec.Version = cfAppRevisionKeyValue
	desiredLRP.Spec.DiskMB = cfProcess.Spec.DiskQuotaMB
	desiredLRP.Spec.MemoryMB = cfProcess.Spec.MemoryMB
	desiredLRP.Spec.ProcessType = cfProcess.Spec.ProcessType
	desiredLRP.Spec.Command = commandForProcess(cfProcess, cfApp)
	desiredLRP.Spec.AppName = cfApp.Spec.DisplayName
	desiredLRP.Spec.AppGUID = cfApp.Name
	desiredLRP.Spec.Image = cfBuild.Status.BuildDropletStatus.Registry.Image
	desiredLRP.Spec.Ports = cfProcess.Spec.Ports
	desiredLRP.Spec.Instances = cfProcess.Spec.DesiredInstances

	desiredLRP.Spec.Env = generateEnvMap(appPort, envVars)
	desiredLRP.Spec.Health = eiriniv1.Healthcheck{
		Type:      string(cfProcess.Spec.HealthCheck.Type),
		Port:      int32(appPort),
		Endpoint:  cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
		TimeoutMs: uint(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds * 1000),
	}
	desiredLRP.Spec.CPUWeight = 0
	desiredLRP.Spec.Sidecars = nil

	err := controllerutil.SetOwnerReference(cfProcess, &desiredLRP, r.Scheme)
	if err != nil {
		return nil, err
	}

	return &desiredLRP, err
}

func generateLRPName(cfAppRev string, processGUID string) string {
	h := sha1.New()
	h.Write([]byte(cfAppRev))
	appRevHash := h.Sum(nil)
	lrpName := processGUID + fmt.Sprintf("-%x", appRevHash)[:5]
	return lrpName
}

func (r *CFProcessReconciler) fetchLRPsForProcess(ctx context.Context, cfProcess *workloadsv1alpha1.CFProcess) ([]eiriniv1.LRP, error) {
	allLRPs := &eiriniv1.LRPList{}
	err := r.Client.List(ctx, allLRPs, client.InNamespace(cfProcess.Namespace))
	if err != nil {
		return []eiriniv1.LRP{}, err
	}
	var lrpsForProcess []eiriniv1.LRP
	for _, currentLRP := range allLRPs.Items {
		if processGUID, has := currentLRP.Labels[workloadsv1alpha1.CFProcessGUIDLabelKey]; has && processGUID == cfProcess.Name {
			lrpsForProcess = append(lrpsForProcess, currentLRP)
		}
	}
	return lrpsForProcess, err
}

func (r *CFProcessReconciler) getPort(ctx context.Context, cfProcess *workloadsv1alpha1.CFProcess, cfApp *workloadsv1alpha1.CFApp) (int, error) {
	// Get Routes for the process
	var cfRoutesForProcess networkingv1alpha1.CFRouteList
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

func generateEnvMap(port int, commonEnv map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range commonEnv {
		result[k] = v
	}

	portString := strconv.Itoa(port)
	result["VCAP_APP_HOST"] = "0.0.0.0"
	result["VCAP_APP_PORT"] = portString
	result["PORT"] = portString

	return result
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
			err := mgr.GetClient().List(context.Background(), processList, client.InNamespace(app.GetNamespace()), client.MatchingLabels{workloadsv1alpha1.CFAppGUIDLabelKey: app.GetName()})
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
