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

package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	TaskCanceledReason    = "TaskCanceled"
	LifecycleLauncherPath = "/cnb/lifecycle/launcher"
)

type TaskEnvBuilder interface {
	Build(context.Context, *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error)
}

type Reconciler struct {
	k8sClient       client.Client
	scheme          *runtime.Scheme
	recorder        record.EventRecorder
	log             logr.Logger
	envBuilder      TaskEnvBuilder
	taskTTLDuration time.Duration
}

func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	log logr.Logger,
	envBuilder TaskEnvBuilder,
	taskTTLDuration time.Duration,
) *k8s.PatchingReconciler[korifiv1alpha1.CFTask] {
	taskReconciler := Reconciler{
		k8sClient:       client,
		scheme:          scheme,
		recorder:        recorder,
		log:             log,
		envBuilder:      envBuilder,
		taskTTLDuration: taskTTLDuration,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFTask](log, client, &taskReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFTask{}).
		Owns(&korifiv1alpha1.TaskWorkload{})
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks/finalizers,verbs=update
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *Reconciler) ReconcileResource(ctx context.Context, cfTask *korifiv1alpha1.CFTask) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	if !cfTask.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	cfTask.Status.ObservedGeneration = cfTask.Generation
	log.V(1).Info("set observed generation", "generation", cfTask.Status.ObservedGeneration)

	if r.alreadyExpired(cfTask) {
		log.V(1).Info("deleting-expired-task", "namespace", cfTask.Namespace, "name", cfTask.Name)
		err := r.k8sClient.Delete(ctx, cfTask)
		if err != nil {
			log.Info("error-deleting-task", "reason", err)
		}
		return ctrl.Result{}, err
	}

	if cfTask.Spec.Canceled {
		err := r.handleCancelation(ctx, cfTask)
		return r.reconcileResult(cfTask, err)
	}

	cfApp, err := r.getApp(ctx, cfTask)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = controllerutil.SetControllerReference(cfApp, cfTask, r.scheme)
	if err != nil {
		log.Info("unable to set owner reference on CFTask", "reason", err)
		return ctrl.Result{}, err
	}

	cfDroplet, err := r.getDroplet(ctx, cfTask, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.initializeStatus(ctx, cfTask, cfDroplet)

	webProcess, err := r.getWebProcess(ctx, cfApp)
	if err != nil {
		log.Info("failed to get web processes", "reason", err)
		return r.reconcileResult(cfTask, err)
	}

	env, err := r.envBuilder.Build(ctx, cfApp)
	if err != nil {
		log.Info("failed to build env", "reason", err)
		return r.reconcileResult(cfTask, err)
	}

	taskWorkload, err := r.createOrPatchTaskWorkload(ctx, cfTask, cfDroplet, webProcess, env)
	if err != nil {
		return r.reconcileResult(cfTask, err)
	}

	r.setTaskStatus(cfTask, taskWorkload.Status.Conditions)

	return r.reconcileResult(cfTask, nil)
}

func (r *Reconciler) setTaskStatus(cfTask *korifiv1alpha1.CFTask, taskWorkloadConditions []metav1.Condition) {
	for _, conditionType := range []string{
		korifiv1alpha1.TaskStartedConditionType,
		korifiv1alpha1.TaskSucceededConditionType,
		korifiv1alpha1.TaskFailedConditionType,
	} {
		cond := meta.FindStatusCondition(taskWorkloadConditions, conditionType)
		if cond == nil {
			continue
		}
		cond.ObservedGeneration = cfTask.Generation
		meta.SetStatusCondition(&cfTask.Status.Conditions, *cond)
	}
}

func (r *Reconciler) getApp(ctx context.Context, cfTask *korifiv1alpha1.CFTask) (*korifiv1alpha1.CFApp, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("getApp").WithValues("appName", cfTask.Spec.AppRef.Name)

	cfApp := new(korifiv1alpha1.CFApp)
	err := r.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cfTask.Namespace,
		Name:      cfTask.Spec.AppRef.Name,
	}, cfApp)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.recorder.Eventf(cfTask, "Warning", "AppNotFound", "Did not find app with name %s in namespace %s", cfTask.Spec.AppRef.Name, cfTask.Namespace)
		} else {
			log.Info("error getting CFApp", "reason", err)
		}

		return nil, err
	}

	if !meta.IsStatusConditionTrue(cfApp.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		log.Info("CFApp not ready",
			"appNamespace", cfApp.Namespace,
			"appName", cfApp.Name,
			"readyCondition", fmt.Sprintf("%+v", meta.FindStatusCondition(cfApp.Status.Conditions, korifiv1alpha1.StatusConditionReady)))
		r.recorder.Eventf(cfTask, "Warning", "AppNotReady", "App %s:%s is not ready", cfApp.Namespace, cfApp.Name)
		return nil, errors.New("app not ready")
	}

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		log.Info("app droplet ref not set")
		r.recorder.Eventf(cfTask, "Warning", "AppCurrentDropletRefNotSet", "App %s does not have a current droplet", cfTask.Spec.AppRef.Name)
		return nil, errors.New("app droplet ref not set")
	}

	return cfApp, nil
}

func (r *Reconciler) getDroplet(ctx context.Context, cfTask *korifiv1alpha1.CFTask, cfApp *korifiv1alpha1.CFApp) (*korifiv1alpha1.CFBuild, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("getDroplet").WithValues("dropletName", cfApp.Spec.CurrentDropletRef.Name)

	cfDroplet := new(korifiv1alpha1.CFBuild)
	err := r.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cfApp.Namespace,
		Name:      cfApp.Spec.CurrentDropletRef.Name,
	}, cfDroplet)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.recorder.Eventf(cfTask, "Warning", "AppCurrentDropletNotFound", "Current droplet %s for app %s does not exist", cfApp.Spec.CurrentDropletRef.Name, cfTask.Spec.AppRef.Name)
		} else {
			log.Info("error getting CFDroplet", "reason", err)
		}

		return nil, err
	}

	if cfDroplet.Status.Droplet == nil {
		log.Info("droplet build status not set")
		r.recorder.Eventf(cfTask, "Warning", "DropletBuildStatusNotSet", "Current droplet %s from app %s does not have a droplet image", cfApp.Spec.CurrentDropletRef.Name, cfTask.Spec.AppRef.Name)
		return nil, errors.New("droplet build status not set")
	}

	return cfDroplet, nil
}

func (r *Reconciler) getWebProcess(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (korifiv1alpha1.CFProcess, error) {
	var processList korifiv1alpha1.CFProcessList
	err := r.k8sClient.List(ctx, &processList, client.InNamespace(cfApp.Namespace), client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
		korifiv1alpha1.CFProcessTypeLabelKey: korifiv1alpha1.ProcessTypeWeb,
	})
	if err != nil {
		return korifiv1alpha1.CFProcess{}, fmt.Errorf("failed to list app processes: %w", err)
	}

	if len(processList.Items) != 1 {
		return korifiv1alpha1.CFProcess{}, fmt.Errorf("expected exactly one web process, found %d", len(processList.Items))
	}

	return processList.Items[0], nil
}

func (r *Reconciler) createOrPatchTaskWorkload(ctx context.Context, cfTask *korifiv1alpha1.CFTask, cfDroplet *korifiv1alpha1.CFBuild, webProcess korifiv1alpha1.CFProcess, env []corev1.EnvVar) (*korifiv1alpha1.TaskWorkload, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("createOrPatchTaskWorkload")

	taskWorkload := &korifiv1alpha1.TaskWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfTask.Name,
			Namespace: cfTask.Namespace,
		},
	}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, taskWorkload, func() error {
		if taskWorkload.Labels == nil {
			taskWorkload.Labels = map[string]string{}
		}

		taskWorkload.Labels[korifiv1alpha1.CFTaskGUIDLabelKey] = cfTask.Name

		taskWorkload.Spec.Command = []string{LifecycleLauncherPath, cfTask.Spec.Command}
		taskWorkload.Spec.Image = cfDroplet.Status.Droplet.Registry.Image
		taskWorkload.Spec.ImagePullSecrets = cfDroplet.Status.Droplet.Registry.ImagePullSecrets

		if taskWorkload.Spec.Resources.Requests == nil {
			taskWorkload.Spec.Resources.Requests = corev1.ResourceList{}
		}

		if taskWorkload.Spec.Resources.Limits == nil {
			taskWorkload.Spec.Resources.Limits = corev1.ResourceList{}
		}

		taskWorkload.Spec.Resources.Requests[corev1.ResourceMemory] = *resource.NewScaledQuantity(cfTask.Status.MemoryMB, resource.Mega)
		taskWorkload.Spec.Resources.Limits[corev1.ResourceMemory] = *resource.NewScaledQuantity(cfTask.Status.MemoryMB, resource.Mega)
		taskWorkload.Spec.Resources.Requests[corev1.ResourceEphemeralStorage] = *resource.NewScaledQuantity(cfTask.Status.DiskQuotaMB, resource.Mega)
		taskWorkload.Spec.Resources.Limits[corev1.ResourceEphemeralStorage] = *resource.NewScaledQuantity(cfTask.Status.DiskQuotaMB, resource.Mega)
		taskWorkload.Spec.Resources.Requests[corev1.ResourceCPU] = *resource.NewScaledQuantity(calculateDefaultCPURequestMillicores(webProcess.Spec.MemoryMB), resource.Milli)
		taskWorkload.Spec.Env = env

		if err := ctrl.SetControllerReference(cfTask, taskWorkload, r.scheme); err != nil {
			log.Info("failed to set owner ref", "reason", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Info("error-creating-or-patching-task-workload", "opResult", opResult, "reason", err)
		return nil, err
	}

	if opResult == controllerutil.OperationResultCreated {
		r.recorder.Eventf(cfTask, "Normal", "TaskWorkloadCreated", "Created task workload %s", taskWorkload.Name)
	}

	return taskWorkload, nil
}

func calculateDefaultCPURequestMillicores(memoryMiB int64) int64 {
	const (
		cpuRequestRatio         int64 = 1024
		cpuRequestMinMillicores int64 = 5
	)

	cpuMillicores := int64(100) * memoryMiB / cpuRequestRatio
	if cpuMillicores < cpuRequestMinMillicores {
		cpuMillicores = cpuRequestMinMillicores
	}

	return cpuMillicores
}

func (r *Reconciler) initializeStatus(ctx context.Context, cfTask *korifiv1alpha1.CFTask, cfDroplet *korifiv1alpha1.CFBuild) {
	cfTask.Status.DropletRef.Name = cfDroplet.Name
	meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.TaskInitializedConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             "TaskInitialized",
		ObservedGeneration: cfTask.Generation,
	})
}

func (r *Reconciler) handleCancelation(ctx context.Context, cfTask *korifiv1alpha1.CFTask) error {
	log := logr.FromContextOrDiscard(ctx).WithName("handleCancelation")

	taskWorkload := &korifiv1alpha1.TaskWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfTask.Name,
			Namespace: cfTask.Namespace,
		},
	}
	err := r.k8sClient.Delete(ctx, taskWorkload)
	if err != nil && !k8serrors.IsNotFound(err) {
		log.Info("error-deleting-task-workload", "reason", err)
		return err
	}

	meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.TaskCanceledConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             TaskCanceledReason,
		ObservedGeneration: cfTask.Generation,
	})

	if !meta.IsStatusConditionTrue(cfTask.Status.Conditions, korifiv1alpha1.TaskSucceededConditionType) {
		meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.TaskFailedConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             TaskCanceledReason,
			ObservedGeneration: cfTask.Generation,
		})
	}

	return nil
}

func (r *Reconciler) reconcileResult(cfTask *korifiv1alpha1.CFTask, reconcileErr error) (ctrl.Result, error) {
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	return ctrl.Result{RequeueAfter: r.computeRequeueAfter(cfTask)}, nil
}

func (r *Reconciler) computeRequeueAfter(cfTask *korifiv1alpha1.CFTask) time.Duration {
	completeTime, isCompleted := getCompletionTime(cfTask)
	if !isCompleted {
		return 0
	}

	return time.Until(completeTime.Add(r.taskTTLDuration))
}

func (r *Reconciler) alreadyExpired(cfTask *korifiv1alpha1.CFTask) bool {
	completeTime, isCompleted := getCompletionTime(cfTask)

	if !isCompleted {
		return false
	}

	return !time.Now().Before(completeTime.Add(r.taskTTLDuration))
}

func getCompletionTime(cfTask *korifiv1alpha1.CFTask) (metav1.Time, bool) {
	succeededCondition := meta.FindStatusCondition(cfTask.Status.Conditions, korifiv1alpha1.TaskSucceededConditionType)
	failedCondition := meta.FindStatusCondition(cfTask.Status.Conditions, korifiv1alpha1.TaskFailedConditionType)

	if succeededCondition == nil && failedCondition == nil {
		return metav1.Time{}, false
	}

	if succeededCondition != nil {
		return succeededCondition.LastTransitionTime, true
	}

	return failedCondition.LastTransitionTime, true
}
