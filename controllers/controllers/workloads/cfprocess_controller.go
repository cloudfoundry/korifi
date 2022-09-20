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
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	LivenessFailureThreshold  = 4
	ReadinessFailureThreshold = 1
)

//counterfeiter:generate -o fake -fake-name EnvBuilder . EnvBuilder
type EnvBuilder interface {
	BuildEnv(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error)
}

// CFProcessReconciler reconciles a CFProcess object
type CFProcessReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Log              logr.Logger
	ControllerConfig *config.ControllerConfig
	EnvBuilder       EnvBuilder
}

func NewCFProcessReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, controllerConfig *config.ControllerConfig, envBuilder EnvBuilder) *CFProcessReconciler {
	return &CFProcessReconciler{Client: client, Scheme: scheme, Log: log, ControllerConfig: controllerConfig, EnvBuilder: envBuilder}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses/finalizers,verbs=update
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

func (r *CFProcessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfProcess := new(korifiv1alpha1.CFProcess)
	var err error
	err = r.Client.Get(ctx, req.NamespacedName, cfProcess)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, fmt.Sprintf("Error when trying to fetch CFProcess %s/%s", req.Namespace, req.Name))
		}
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
		err = r.createOrPatchAppWorkload(ctx, cfApp, cfProcess, cfAppRev)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.cleanUpAppWorkloads(ctx, cfProcess, cfApp.Spec.DesiredState, cfAppRev)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFProcessReconciler) createOrPatchAppWorkload(ctx context.Context, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess, cfAppRev string) error {
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

	actualAppWorkload := &korifiv1alpha1.AppWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfProcess.Namespace,
			Name:      generateAppWorkloadName(cfAppRev, cfProcess.Name),
		},
	}

	var desiredAppWorkload *korifiv1alpha1.AppWorkload
	desiredAppWorkload, err = r.generateAppWorkload(actualAppWorkload, cfApp, cfProcess, cfBuild, appPort, envVars)
	if err != nil { // untested
		r.Log.Error(err, "Error when initializing AppWorkload")
		return err
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, actualAppWorkload, appWorkloadMutateFunction(actualAppWorkload, desiredAppWorkload))
	if err != nil {
		r.Log.Error(err, "Error calling CreateOrPatch on AppWorkload")
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

func (r *CFProcessReconciler) cleanUpAppWorkloads(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess, desiredState korifiv1alpha1.DesiredState, cfAppRev string) error {
	appWorkloadsForProcess, err := r.fetchAppWorkloadsForProcess(ctx, cfProcess)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error when trying to fetch AppWorkloads for Process %s/%s", cfProcess.Namespace, cfProcess.Name))
		return err
	}

	for i, currentAppWorkload := range appWorkloadsForProcess {
		if desiredState == korifiv1alpha1.StoppedState || currentAppWorkload.Labels[korifiv1alpha1.CFAppRevisionKey] != cfAppRev {
			err := r.Client.Delete(ctx, &appWorkloadsForProcess[i])
			if err != nil {
				r.Log.Info(fmt.Sprintf("Error occurred deleting AppWorkload: %s, %s", currentAppWorkload.Name, err))
				return err
			}
		}
	}
	return nil
}

func appWorkloadMutateFunction(actualAppWorkload, desiredAppWorkload *korifiv1alpha1.AppWorkload) controllerutil.MutateFn {
	return func() error {
		actualAppWorkload.ObjectMeta.Labels = desiredAppWorkload.ObjectMeta.Labels
		actualAppWorkload.ObjectMeta.Annotations = desiredAppWorkload.ObjectMeta.Annotations
		actualAppWorkload.ObjectMeta.OwnerReferences = desiredAppWorkload.ObjectMeta.OwnerReferences
		actualAppWorkload.Spec = desiredAppWorkload.Spec
		return nil
	}
}

func (r *CFProcessReconciler) generateAppWorkload(actualAppWorkload *korifiv1alpha1.AppWorkload, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess, cfBuild *korifiv1alpha1.CFBuild, appPort int, envVars []corev1.EnvVar) (*korifiv1alpha1.AppWorkload, error) {
	var desiredAppWorkload korifiv1alpha1.AppWorkload
	actualAppWorkload.DeepCopyInto(&desiredAppWorkload)

	desiredAppWorkload.Labels = make(map[string]string)
	desiredAppWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey] = cfApp.Name
	cfAppRevisionKeyValue := korifiv1alpha1.CFAppRevisionKeyDefault
	if cfApp.Annotations != nil {
		if foundValue, has := cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey]; has {
			cfAppRevisionKeyValue = foundValue
		}
	}
	desiredAppWorkload.Labels[korifiv1alpha1.CFAppRevisionKey] = cfAppRevisionKeyValue
	desiredAppWorkload.Labels[korifiv1alpha1.CFProcessGUIDLabelKey] = cfProcess.Name
	desiredAppWorkload.Labels[korifiv1alpha1.CFProcessTypeLabelKey] = cfProcess.Spec.ProcessType

	desiredAppWorkload.Spec.GUID = cfProcess.Name
	desiredAppWorkload.Spec.Version = cfAppRevisionKeyValue
	desiredAppWorkload.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU:              calculateCPURequest(cfProcess.Spec.MemoryMB),
		corev1.ResourceEphemeralStorage: mebibyteQuantity(cfProcess.Spec.DiskQuotaMB),
		corev1.ResourceMemory:           mebibyteQuantity(cfProcess.Spec.MemoryMB),
	}
	desiredAppWorkload.Spec.Resources.Limits = corev1.ResourceList{
		corev1.ResourceEphemeralStorage: mebibyteQuantity(cfProcess.Spec.DiskQuotaMB),
		corev1.ResourceMemory:           mebibyteQuantity(cfProcess.Spec.MemoryMB),
	}
	desiredAppWorkload.Spec.ProcessType = cfProcess.Spec.ProcessType
	desiredAppWorkload.Spec.Command = commandForProcess(cfProcess, cfApp)
	desiredAppWorkload.Spec.AppGUID = cfApp.Name
	desiredAppWorkload.Spec.Image = cfBuild.Status.Droplet.Registry.Image
	desiredAppWorkload.Spec.ImagePullSecrets = cfBuild.Status.Droplet.Registry.ImagePullSecrets
	desiredAppWorkload.Spec.Ports = cfProcess.Spec.Ports
	if cfProcess.Spec.DesiredInstances != nil {
		desiredAppWorkload.Spec.Instances = int32(*cfProcess.Spec.DesiredInstances)
	}

	desiredAppWorkload.Spec.Env = generateEnvVars(appPort, envVars)
	desiredAppWorkload.Spec.LivenessProbe = createLivenessProbe(cfProcess, int32(appPort))
	desiredAppWorkload.Spec.ReadinessProbe = createReadinessProbe(cfProcess, int32(appPort))
	desiredAppWorkload.Spec.RunnerName = r.ControllerConfig.RunnerName

	err := controllerutil.SetOwnerReference(cfProcess, &desiredAppWorkload, r.Scheme)
	if err != nil {
		return nil, err
	}

	return &desiredAppWorkload, err
}

func calculateCPURequest(memoryMiB int64) resource.Quantity {
	const (
		cpuRequestRatio         int64 = 1024
		cpuRequestMinMillicores int64 = 5
	)
	cpuMillicores := int64(100) * memoryMiB / cpuRequestRatio
	if cpuMillicores < cpuRequestMinMillicores {
		cpuMillicores = cpuRequestMinMillicores
	}
	return *resource.NewScaledQuantity(cpuMillicores, resource.Milli)
}

func generateAppWorkloadName(cfAppRev string, processGUID string) string {
	h := sha1.New()
	h.Write([]byte(cfAppRev))
	appRevHash := h.Sum(nil)
	appWorkloadName := processGUID + fmt.Sprintf("-%x", appRevHash)[:5]
	return appWorkloadName
}

func (r *CFProcessReconciler) fetchAppWorkloadsForProcess(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess) ([]korifiv1alpha1.AppWorkload, error) {
	allAppWorkloads := &korifiv1alpha1.AppWorkloadList{}
	err := r.Client.List(ctx, allAppWorkloads, client.InNamespace(cfProcess.Namespace))
	if err != nil {
		return []korifiv1alpha1.AppWorkload{}, err
	}
	var appWorkloadsForProcess []korifiv1alpha1.AppWorkload
	for _, currentAppWorkload := range allAppWorkloads.Items {
		if processGUID, has := currentAppWorkload.Labels[korifiv1alpha1.CFProcessGUIDLabelKey]; has && processGUID == cfProcess.Name {
			appWorkloadsForProcess = append(appWorkloadsForProcess, currentAppWorkload)
		}
	}
	return appWorkloadsForProcess, err
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

func createLivenessProbe(cfProcess *korifiv1alpha1.CFProcess, port int32) *corev1.Probe {
	initialDelay := int32(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds)
	healthCheckType := string(cfProcess.Spec.HealthCheck.Type)

	if healthCheckType == "http" {
		return createHTTPProbe(cfProcess.Spec.HealthCheck.Data.HTTPEndpoint, port, initialDelay, LivenessFailureThreshold)
	}

	if healthCheckType == "port" {
		return createPortProbe(port, initialDelay, LivenessFailureThreshold)
	}

	return nil
}

func createReadinessProbe(cfProcess *korifiv1alpha1.CFProcess, port int32) *corev1.Probe {
	healthCheckType := string(cfProcess.Spec.HealthCheck.Type)

	if healthCheckType == "http" {
		return createHTTPProbe(cfProcess.Spec.HealthCheck.Data.HTTPEndpoint, port, 0, ReadinessFailureThreshold)
	}

	if healthCheckType == "port" {
		return createPortProbe(port, 0, ReadinessFailureThreshold)
	}

	return nil
}

func createPortProbe(port, initialDelay, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: tcpSocketAction(port),
		},
		InitialDelaySeconds: initialDelay,
		FailureThreshold:    failureThreshold,
	}
}

func createHTTPProbe(endpoint string, port, initialDelay, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: httpGetAction(endpoint, port),
		},
		InitialDelaySeconds: initialDelay,
		FailureThreshold:    failureThreshold,
	}
}

func httpGetAction(endpoint string, port int32) *corev1.HTTPGetAction {
	return &corev1.HTTPGetAction{
		Path: endpoint,
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: port},
	}
}

func tcpSocketAction(port int32) *corev1.TCPSocketAction {
	return &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: port},
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
			for i := range processList.Items {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&processList.Items[i])})
			}

			return requests
		})).
		Complete(r)
}

func mebibyteQuantity(miB int64) resource.Quantity {
	return *resource.NewQuantity(miB*1024*1024, resource.BinarySI)
}
