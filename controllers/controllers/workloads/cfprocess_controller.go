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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/shared"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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

	servicebindingv1alpha3 "github.com/servicebinding/service-binding-controller/apis/v1alpha3"
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
//+kubebuilder:rbac:groups=servicebinding.io,resources=*,verbs=get;list;watch

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

	cfAppRev := workloadsv1alpha1.CFAppRevisionKeyDefault
	if foundValue, ok := cfApp.GetAnnotations()[workloadsv1alpha1.CFAppRevisionKey]; ok {
		cfAppRev = foundValue
	}
	lrpsForProcess, err := r.fetchLRPsForProcess(ctx, cfProcess)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch LRPs for Process %s/%s", req.Namespace, cfProcess.Name))
		return ctrl.Result{}, err
	}
	if cfApp.Spec.DesiredState == workloadsv1alpha1.StartedState {

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

		var appPort int
		appPort, err = r.getPort(ctx, cfProcess, cfApp)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Error when trying to fetch routes for CFApp %s/%s", req.Namespace, cfApp.Spec.Name))
			return ctrl.Result{}, err
		}

		actualLRP := &eiriniv1.LRP{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfProcess.Namespace,
				Name:      generateLRPName(cfAppRev, cfProcess.Name),
			},
		}

		vcapService, err := r.getServiceBindings(ctx, cfProcess.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}

		var desiredLRP *eiriniv1.LRP
		desiredLRP, err = r.generateLRP(*actualLRP, cfApp, cfProcess, cfBuild, appEnvSecret, appPort, vcapService)
		if err != nil {
			// untested
			r.Log.Error(err, "Error when initializing LRP")
			return ctrl.Result{}, err
		}

		_, err = controllerutil.CreateOrPatch(ctx, r.Client, actualLRP, lrpMutateFunction(actualLRP, desiredLRP))
		if err != nil {
			r.Log.Error(err, "Error calling CreateOrPatch on lrp")
			return ctrl.Result{}, err
		}
	}

	// Cleanup LRPs
	err = r.cleanUpLRPs(ctx, lrpsForProcess, cfApp.Spec.DesiredState, cfAppRev)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFProcessReconciler) getServiceBindings(ctx context.Context, namespace string) (string, error) {
	sbList := servicebindingv1alpha3.ServiceBindingList{}
	err := r.Client.List(ctx, &sbList, &client.ListOptions{Namespace: namespace})
	if err != nil {
		r.Log.Error(err, "Error listing service bindings")
		return "", err
	}

	if len(sbList.Items) == 0 {
		return "{}", nil
	}

	serviceBindingSecret := corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: sbList.Items[0].Status.Binding.Name, Namespace: namespace}, &serviceBindingSecret)
	if err != nil {
		r.Log.Error(err, "Error fetching secret associated with service binding")
		return "", err
	}

	return r.getVCAPSERVICEEnvJson(sbList.Items[0], serviceBindingSecret), nil
}

func (r *CFProcessReconciler) getVCAPSERVICEEnvJson(servicebinding servicebindingv1alpha3.ServiceBinding, secret corev1.Secret) string {
	convertedMap := make(map[string]string)
	for k, v := range secret.Data {
		convertedMap[k] = string(v)
	}

	vcapservice := map[string]interface{}{
		"name":        servicebinding.Name,
		"credentials": convertedMap,
	}

	vcapservicebyte, err := json.Marshal(vcapservice)
	if err != nil {
		r.Log.Error(err, "Error marshalling json")
	}
	return string(vcapservicebyte)
}

func (r *CFProcessReconciler) cleanUpLRPs(ctx context.Context, lrpsForProcess []eiriniv1.LRP, desiredState workloadsv1alpha1.DesiredState, cfAppRev string) error {
	for _, currentLRP := range lrpsForProcess {
		toDelete := true
		if desiredState != workloadsv1alpha1.StoppedState {
			if getMapKeyValue(currentLRP.Labels, workloadsv1alpha1.CFAppRevisionKey) == cfAppRev {
				toDelete = false
			}
		}

		if toDelete {
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

func (r *CFProcessReconciler) generateLRP(actualLRP eiriniv1.LRP, cfApp workloadsv1alpha1.CFApp, cfProcess workloadsv1alpha1.CFProcess, cfBuild workloadsv1alpha1.CFBuild, appEnvSecret corev1.Secret, appPort int, vcapservice string) (*eiriniv1.LRP, error) {
	var desiredLRP eiriniv1.LRP
	actualLRP.DeepCopyInto(&desiredLRP)

	var lrpHealthCheckPort int32 = 0
	if len(cfProcess.Spec.Ports) > 0 {
		lrpHealthCheckPort = cfProcess.Spec.Ports[0]
	}

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
	desiredLRP.Spec.Command = commandForProcess(&cfProcess, &cfApp)
	desiredLRP.Spec.AppName = cfApp.Spec.Name
	desiredLRP.Spec.AppGUID = cfApp.Name
	desiredLRP.Spec.Image = cfBuild.Status.BuildDropletStatus.Registry.Image
	desiredLRP.Spec.Ports = cfProcess.Spec.Ports
	desiredLRP.Spec.Instances = cfProcess.Spec.DesiredInstances
	desiredLRP.Spec.Env = generateEnvMap(appEnvSecret.Data, appPort, vcapservice)
	desiredLRP.Spec.Health = eiriniv1.Healthcheck{
		Type:      string(cfProcess.Spec.HealthCheck.Type),
		Port:      lrpHealthCheckPort,
		Endpoint:  cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
		TimeoutMs: uint(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds * 1000),
	}
	desiredLRP.Spec.CPUWeight = 0
	desiredLRP.Spec.Sidecars = nil

	err := controllerutil.SetOwnerReference(&cfProcess, &desiredLRP, r.Scheme)
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

func (r *CFProcessReconciler) fetchLRPsForProcess(ctx context.Context, cfProcess workloadsv1alpha1.CFProcess) ([]eiriniv1.LRP, error) {
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

func (r *CFProcessReconciler) getPort(ctx context.Context, cfProcess workloadsv1alpha1.CFProcess, cfApp workloadsv1alpha1.CFApp) (int, error) {
	// Get Routes for the process
	var cfRoutesForProcess networkingv1alpha1.CFRouteList
	err := r.Client.List(ctx, &cfRoutesForProcess, client.InNamespace(cfApp.GetNamespace()), client.MatchingFields{shared.DestinationAppName: cfApp.Name})
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

func generateEnvMap(secretData map[string][]byte, port int, vcapservice string) map[string]string {
	convertedMap := make(map[string]string)
	for k, v := range secretData {
		convertedMap[k] = string(v)
	}

	portString := strconv.Itoa(port)
	convertedMap["VCAP_APP_HOST"] = "0.0.0.0"
	convertedMap["VCAP_APP_PORT"] = portString
	convertedMap["PORT"] = portString
	convertedMap["VCAP_SERVICES"] = vcapservice

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
