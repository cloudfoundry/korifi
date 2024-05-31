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

package processes

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/ports"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ProcessEnvBuilder interface {
	Build(context.Context, *korifiv1alpha1.CFApp, *korifiv1alpha1.CFProcess) ([]corev1.EnvVar, error)
}

type Reconciler struct {
	k8sClient        client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	controllerConfig *config.ControllerConfig
	envBuilder       ProcessEnvBuilder
}

func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	controllerConfig *config.ControllerConfig,
	envBuilder ProcessEnvBuilder,
) *k8s.PatchingReconciler[korifiv1alpha1.CFProcess, *korifiv1alpha1.CFProcess] {
	processReconciler := Reconciler{k8sClient: client, scheme: scheme, log: log, controllerConfig: controllerConfig, envBuilder: envBuilder}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFProcess, *korifiv1alpha1.CFProcess](log, client, &processReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFProcess{}).
		Owns(&korifiv1alpha1.AppWorkload{}).
		Watches(
			&korifiv1alpha1.CFApp{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFProcessRequestsForApp),
		).
		Watches(
			&korifiv1alpha1.CFRoute{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFProcessRequestsForRoute),
		)
}

func (r *Reconciler) enqueueCFProcessRequestsForApp(ctx context.Context, o client.Object) []reconcile.Request {
	return r.cfProcessRequestsForAppGUID(ctx, o.GetNamespace(), o.GetName())
}

func (r *Reconciler) cfProcessRequestsForAppGUID(ctx context.Context, cfAppNamespace, cfAppGUID string) []reconcile.Request {
	processList := &korifiv1alpha1.CFProcessList{}
	err := r.k8sClient.List(ctx, processList, client.InNamespace(cfAppNamespace), client.MatchingLabels{korifiv1alpha1.CFAppGUIDLabelKey: cfAppGUID})
	if err != nil {
		r.log.Error(fmt.Errorf("listing CFProcesses for CFApp guid failed: %w", err), "cfAppGUID", cfAppGUID)
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for i := range processList.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&processList.Items[i])})
	}

	return requests
}

func (r *Reconciler) enqueueCFProcessRequestsForRoute(ctx context.Context, o client.Object) []reconcile.Request {
	cfRoute, ok := o.(*korifiv1alpha1.CFRoute)
	if !ok {
		r.log.Error(errors.New("listing CFProcesses for route failed"), "expected", "CFRoute", "got", o)
		return []reconcile.Request{}
	}

	result := []reconcile.Request{}
	for _, destination := range cfRoute.Status.Destinations {
		result = append(result, r.cfProcessRequestsForAppGUID(ctx, cfRoute.Namespace, destination.AppRef.Name)...)
	}

	return result
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfprocesses/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

func (r *Reconciler) ReconcileResource(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	var err error
	readyConditionBuilder := k8s.NewReadyConditionBuilder(cfProcess)
	defer func() {
		meta.SetStatusCondition(&cfProcess.Status.Conditions, readyConditionBuilder.WithError(err).Build())
	}()

	cfProcess.Status.ObservedGeneration = cfProcess.Generation
	log.V(1).Info("set observed generation", "generation", cfProcess.Status.ObservedGeneration)

	shared.GetConditionOrSetAsUnknown(&cfProcess.Status.Conditions, korifiv1alpha1.StatusConditionReady, cfProcess.Generation)

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfProcess.Spec.AppRef.Name, Namespace: cfProcess.Namespace}, cfApp)
	if err != nil {
		log.Info("error when trying to fetch CFApp", "namespace", cfProcess.Namespace, "name", cfProcess.Spec.AppRef.Name, "reason", err)
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(cfApp, cfProcess, r.scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfAppRev := korifiv1alpha1.CFAppRevisionKeyDefault
	if foundValue, ok := cfApp.GetAnnotations()[korifiv1alpha1.CFAppRevisionKey]; ok {
		cfAppRev = foundValue
	}

	cfLastStopAppRev := cfAppRev
	if foundValue, ok := cfApp.GetAnnotations()[korifiv1alpha1.CFAppLastStopRevisionKey]; ok {
		cfLastStopAppRev = foundValue
	}

	if needsAppWorkload(cfApp, cfProcess) {
		err = r.createOrPatchAppWorkload(ctx, cfApp, cfProcess, cfAppRev, cfLastStopAppRev)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.cleanUpAppWorkloads(ctx, cfProcess, cfApp.Spec.DesiredState, cfLastStopAppRev)
	if err != nil {
		return ctrl.Result{}, err
	}

	appWorkloads, err := r.fetchAppWorkloadsForProcess(ctx, cfProcess)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfProcess.Status.ActualInstances = getActualInstances(appWorkloads)

	readyConditionBuilder.Ready()
	return ctrl.Result{}, nil
}

func getActualInstances(appWorkloads []korifiv1alpha1.AppWorkload) int32 {
	actualInstances := int32(0)
	for _, w := range appWorkloads {
		actualInstances += w.Status.ActualInstances
	}
	return actualInstances
}

func needsAppWorkload(cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess) bool {
	if cfApp.Spec.DesiredState != korifiv1alpha1.StartedState {
		return false
	}

	// note that the defaulting webhook ensures DesiredInstances is never nil
	return cfProcess.Spec.DesiredInstances != nil && *cfProcess.Spec.DesiredInstances > 0
}

func (r *Reconciler) createOrPatchAppWorkload(ctx context.Context, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess, cfAppRev, cfLastStopAppRev string) error {
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchAppWorkload")

	cfBuild := new(korifiv1alpha1.CFBuild)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfProcess.Namespace}, cfBuild)
	if err != nil {
		log.Info("error when trying to fetch CFBuild", "namespace", cfProcess.Namespace, "name", cfApp.Spec.CurrentDropletRef.Name, "reason", err)
		return err
	}

	if cfBuild.Status.Droplet == nil {
		log.Info("no build droplet status on CFBuild", "namespace", cfProcess.Namespace, "name", cfApp.Spec.CurrentDropletRef.Name, "reason", err)
		return errors.New("no build droplet status on CFBuild")
	}

	var cfRoutesForProcess korifiv1alpha1.CFRouteList
	err = r.k8sClient.List(ctx, &cfRoutesForProcess,
		client.InNamespace(cfProcess.Namespace),
		client.MatchingFields{shared.IndexRouteDestinationAppName: cfApp.Name},
	)
	if err != nil {
		return err
	}

	appPorts := ports.FromRoutes(cfRoutesForProcess.Items, cfApp.Name, cfProcess.Spec.ProcessType)

	envVars, err := r.envBuilder.Build(ctx, cfApp, cfProcess)
	if err != nil {
		log.Info("error when trying build the process environment for app", "namespace", cfProcess.Namespace, "name", cfApp.Spec.DisplayName, "reason", err)
		return err
	}

	actualAppWorkload := &korifiv1alpha1.AppWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfProcess.Namespace,
			Name:      generateAppWorkloadName(cfLastStopAppRev, cfProcess.Name),
		},
	}

	var desiredAppWorkload *korifiv1alpha1.AppWorkload
	desiredAppWorkload, err = r.generateAppWorkload(actualAppWorkload, cfApp, cfProcess, cfBuild, appPorts, envVars, cfAppRev, cfLastStopAppRev)
	if err != nil { // untested
		log.Info("error when initializing AppWorkload", "reason", err)
		return err
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, actualAppWorkload, appWorkloadMutateFunction(actualAppWorkload, desiredAppWorkload))
	if err != nil {
		log.Info("error calling CreateOrPatch on AppWorkload", "reason", err)
		return err
	}
	return nil
}

func (r *Reconciler) cleanUpAppWorkloads(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess, desiredState korifiv1alpha1.AppState, cfLastStopAppRev string) error {
	log := logr.FromContextOrDiscard(ctx).WithName("cleanUpAppWorkloads")

	appWorkloadsForProcess, err := r.fetchAppWorkloadsForProcess(ctx, cfProcess)
	if err != nil {
		log.Info("error when trying to fetch AppWorkloads for process", "namespace", cfProcess.Namespace, "name", cfProcess.Name, "reason", err)
		return err
	}

	for i, currentAppWorkload := range appWorkloadsForProcess {
		if needsToDeleteAppWorkload(desiredState, cfProcess, currentAppWorkload, cfLastStopAppRev) {
			err := r.k8sClient.Delete(ctx, &appWorkloadsForProcess[i])
			if err != nil {
				log.Info("error occurred deleting AppWorkload", "name", currentAppWorkload.Name, "reason", err)
				return err
			}
		}
	}
	return nil
}

func needsToDeleteAppWorkload(
	desiredState korifiv1alpha1.AppState,
	cfProcess *korifiv1alpha1.CFProcess,
	appWorkload korifiv1alpha1.AppWorkload,
	cfLastStopAppRev string,
) bool {
	return desiredState == korifiv1alpha1.StoppedState ||
		(cfProcess.Spec.DesiredInstances != nil && *cfProcess.Spec.DesiredInstances == 0) ||
		appWorkload.Name != generateAppWorkloadName(cfLastStopAppRev, cfProcess.Name)
}

func appWorkloadMutateFunction(actualAppWorkload, desiredAppWorkload *korifiv1alpha1.AppWorkload) controllerutil.MutateFn {
	return func() error {
		actualAppWorkload.Labels = desiredAppWorkload.Labels
		actualAppWorkload.Annotations = desiredAppWorkload.Annotations
		actualAppWorkload.OwnerReferences = desiredAppWorkload.OwnerReferences
		actualAppWorkload.Spec = desiredAppWorkload.Spec
		return nil
	}
}

func (r *Reconciler) generateAppWorkload(actualAppWorkload *korifiv1alpha1.AppWorkload, cfApp *korifiv1alpha1.CFApp, cfProcess *korifiv1alpha1.CFProcess, cfBuild *korifiv1alpha1.CFBuild, appPorts []int32, envVars []corev1.EnvVar, cfAppRev, cfLastStopAppRev string) (*korifiv1alpha1.AppWorkload, error) {
	var desiredAppWorkload korifiv1alpha1.AppWorkload
	actualAppWorkload.DeepCopyInto(&desiredAppWorkload)

	desiredAppWorkload.Labels = make(map[string]string)
	desiredAppWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey] = cfApp.Name
	desiredAppWorkload.Labels[korifiv1alpha1.CFAppRevisionKey] = cfAppRev
	desiredAppWorkload.Labels[korifiv1alpha1.CFProcessGUIDLabelKey] = cfProcess.Name
	desiredAppWorkload.Labels[korifiv1alpha1.CFProcessTypeLabelKey] = cfProcess.Spec.ProcessType

	desiredAppWorkload.Annotations = make(map[string]string)
	desiredAppWorkload.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = cfLastStopAppRev

	desiredAppWorkload.Spec.GUID = cfProcess.Name
	desiredAppWorkload.Spec.Version = cfAppRev
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

	desiredAppWorkload.Spec.Ports = appPorts
	if cfProcess.Spec.DesiredInstances != nil {
		desiredAppWorkload.Spec.Instances = int32(*cfProcess.Spec.DesiredInstances)
	}

	desiredAppWorkload.Spec.Env = envVars

	desiredAppWorkload.Spec.StartupProbe = startupProbe(cfProcess, appPorts)
	desiredAppWorkload.Spec.LivenessProbe = livenessProbe(cfProcess, appPorts)
	desiredAppWorkload.Spec.RunnerName = r.controllerConfig.RunnerName

	err := controllerutil.SetControllerReference(cfProcess, &desiredAppWorkload, r.scheme)
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

func (r *Reconciler) fetchAppWorkloadsForProcess(ctx context.Context, cfProcess *korifiv1alpha1.CFProcess) ([]korifiv1alpha1.AppWorkload, error) {
	allAppWorkloads := &korifiv1alpha1.AppWorkloadList{}
	err := r.k8sClient.List(ctx, allAppWorkloads, client.InNamespace(cfProcess.Namespace))
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

func commandForProcess(process *korifiv1alpha1.CFProcess, app *korifiv1alpha1.CFApp) []string {
	cmd := process.Spec.Command
	if cmd == "" {
		cmd = process.Spec.DetectedCommand
	}

	if cmd == "" {
		return []string{}
	}

	if app.Spec.Lifecycle.Type == korifiv1alpha1.BuildpackLifecycle {
		return []string{"/cnb/lifecycle/launcher", cmd}
	}

	return []string{"/bin/sh", "-c", cmd}
}

func makeProbeHandler(cfProcess *korifiv1alpha1.CFProcess, port int32) corev1.ProbeHandler {
	var probeHandler corev1.ProbeHandler

	switch cfProcess.Spec.HealthCheck.Type {
	case korifiv1alpha1.HTTPHealthCheckType:
		probeHandler.HTTPGet = &corev1.HTTPGetAction{
			Path: cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
			Port: intstr.FromInt32(port),
		}
	case korifiv1alpha1.PortHealthCheckType:
		probeHandler.TCPSocket = &corev1.TCPSocketAction{
			Port: intstr.FromInt32(port),
		}
	}

	return probeHandler
}

func startupProbe(cfProcess *korifiv1alpha1.CFProcess, ports []int32) *corev1.Probe {
	if cfProcess.Spec.HealthCheck.Type == korifiv1alpha1.ProcessHealthCheckType {
		return nil
	}

	if len(ports) == 0 {
		return nil
	}

	return &corev1.Probe{
		ProbeHandler:   makeProbeHandler(cfProcess, ports[0]),
		TimeoutSeconds: int32(cfProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds),
		PeriodSeconds:  2,
		FailureThreshold: int32(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds/2 +
			(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds)%2),
	}
}

func livenessProbe(cfProcess *korifiv1alpha1.CFProcess, ports []int32) *corev1.Probe {
	if cfProcess.Spec.HealthCheck.Type == korifiv1alpha1.ProcessHealthCheckType {
		return nil
	}

	if len(ports) == 0 {
		return nil
	}

	return &corev1.Probe{
		ProbeHandler:     makeProbeHandler(cfProcess, ports[0]),
		TimeoutSeconds:   int32(cfProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds),
		PeriodSeconds:    30,
		FailureThreshold: 1,
	}
}

func mebibyteQuantity(miB int64) resource.Quantity {
	return *resource.NewQuantity(miB*1024*1024, resource.BinarySI)
}
