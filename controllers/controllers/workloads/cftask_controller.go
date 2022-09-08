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
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/tools/k8s"
)

const (
	TaskCanceledReason    = "taskCanceled"
	LifecycleLauncherPath = "/cnb/lifecycle/launcher"
)

//counterfeiter:generate -o fake -fake-name SeqIdGenerator . SeqIdGenerator
type SeqIdGenerator interface {
	Generate() (int64, error)
}

// CFTaskReconciler reconciles a CFTask object
type CFTaskReconciler struct {
	k8sClient         client.Client
	scheme            *runtime.Scheme
	recorder          record.EventRecorder
	logger            logr.Logger
	seqIdGenerator    SeqIdGenerator
	envBuilder        EnvBuilder
	cfProcessDefaults config.CFProcessDefaults
	taskTTLDuration   time.Duration
}

func NewCFTaskReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	logger logr.Logger,
	seqIdGenerator SeqIdGenerator,
	envBuilder EnvBuilder,
	cfProcessDefaults config.CFProcessDefaults,
	taskTTLDuration time.Duration,
) *CFTaskReconciler {
	return &CFTaskReconciler{
		k8sClient:         client,
		scheme:            scheme,
		recorder:          recorder,
		logger:            logger,
		seqIdGenerator:    seqIdGenerator,
		envBuilder:        envBuilder,
		cfProcessDefaults: cfProcessDefaults,
		taskTTLDuration:   taskTTLDuration,
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks/finalizers,verbs=update
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *CFTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	cfTask := new(korifiv1alpha1.CFTask)
	if err := r.k8sClient.Get(ctx, req.NamespacedName, cfTask); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.logger.Info("error-getting-cftask", "error", err)
		return ctrl.Result{}, err
	}

	if r.alreadyExpired(cfTask) {
		r.logger.Info("deleting-expired-task", "namespace", cfTask.Namespace, "name", cfTask.Name)
		err := r.k8sClient.Delete(ctx, cfTask)
		if err != nil {
			r.logger.Error(err, "error-deleting-task")
		}
		return ctrl.Result{}, err
	}

	if cfTask.Spec.Canceled {
		err := r.handleCancelation(ctx, cfTask)
		return r.updateStatusAndReturn(ctx, cfTask, err)
	}

	cfApp, err := r.getApp(ctx, cfTask)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfDroplet, err := r.getDroplet(ctx, cfTask, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ensureInitialized(ctx, cfTask, cfDroplet)
	if err != nil {
		return ctrl.Result{}, err
	}

	webProcess, err := r.getWebProcess(ctx, cfApp)
	if err != nil {
		r.logger.Error(err, "failed to get web processes")
		return r.updateStatusAndReturn(ctx, cfTask, err)
	}

	env, err := r.envBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		r.logger.Error(err, "failed to build env")
		return r.updateStatusAndReturn(ctx, cfTask, err)
	}

	taskWorkload, err := r.createOrPatchTaskWorkload(ctx, cfTask, cfDroplet, webProcess, env)
	if err != nil {
		return r.updateStatusAndReturn(ctx, cfTask, err)
	}

	err = k8s.PatchStatusConditions(ctx, r.k8sClient, cfTask, filterConditions(taskWorkload.Status.Conditions)...)
	return ctrl.Result{}, err
}

func filterConditions(objConditions []metav1.Condition) []metav1.Condition {
	result := []metav1.Condition{}
	for _, cond := range objConditions {
		switch cond.Type {
		case korifiv1alpha1.TaskStartedConditionType:
			fallthrough
		case korifiv1alpha1.TaskSucceededConditionType:
			fallthrough
		case korifiv1alpha1.TaskFailedConditionType:
			result = append(result, cond)
		}
	}

	return result
}

func (r *CFTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFTask{}).
		Owns(&korifiv1alpha1.TaskWorkload{}).
		Complete(r)
}

func (r *CFTaskReconciler) getApp(ctx context.Context, cfTask *korifiv1alpha1.CFTask) (*korifiv1alpha1.CFApp, error) {
	cfApp := new(korifiv1alpha1.CFApp)
	err := r.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cfTask.Namespace,
		Name:      cfTask.Spec.AppRef.Name,
	}, cfApp)
	if err != nil {
		r.logger.Info("error-getting-cfapp", "error", err)
		if k8serrors.IsNotFound(err) {
			r.recorder.Eventf(cfTask, "Warning", "appNotFound", "Did not find app with name %s in namespace %s", cfTask.Spec.AppRef.Name, cfTask.Namespace)
		}
		return nil, err
	}

	if !meta.IsStatusConditionTrue(cfApp.Status.Conditions, StatusConditionStaged) {
		r.logger.Info("cfapp not staged", "app-namespace", cfApp.Namespace, "app-name", cfApp.Name)
		r.recorder.Eventf(cfTask, "Warning", "appNotStaged", "App %s:%s is not staged", cfApp.Namespace, cfApp.Name)
		return nil, errors.New("app not staged")
	}

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		r.recorder.Eventf(cfTask, "Warning", "appCurrentDropletRefNotSet", "App %s does not have a current droplet", cfTask.Spec.AppRef.Name)
		return nil, errors.New("app droplet ref not set")
	}

	return cfApp, nil
}

func (r *CFTaskReconciler) getDroplet(ctx context.Context, cfTask *korifiv1alpha1.CFTask, cfApp *korifiv1alpha1.CFApp) (*korifiv1alpha1.CFBuild, error) {
	cfDroplet := new(korifiv1alpha1.CFBuild)
	err := r.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cfApp.Namespace,
		Name:      cfApp.Spec.CurrentDropletRef.Name,
	}, cfDroplet)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.recorder.Eventf(cfTask, "Warning", "appCurrentDropletNotFound", "Current droplet %s for app %s does not exist", cfApp.Spec.CurrentDropletRef.Name, cfTask.Spec.AppRef.Name)
		}
		r.logger.Info("error-getting-cfdroplet", "error", err)
		return nil, err
	}

	if cfDroplet.Status.Droplet == nil {
		r.recorder.Eventf(cfTask, "Warning", "dropletBuildStatusNotSet", "Current droplet %s from app %s does not have a droplet image", cfApp.Spec.CurrentDropletRef.Name, cfTask.Spec.AppRef.Name)
		return nil, errors.New("droplet build status not set")
	}

	return cfDroplet, nil
}

func (r *CFTaskReconciler) getWebProcess(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (korifiv1alpha1.CFProcess, error) {
	var processList korifiv1alpha1.CFProcessList
	err := r.k8sClient.List(ctx, &processList, client.InNamespace(cfApp.Namespace), client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
		korifiv1alpha1.CFProcessTypeLabelKey: "web",
	})
	if err != nil {
		r.logger.Error(err, "failed to list app processes")
		return korifiv1alpha1.CFProcess{}, err
	}
	if len(processList.Items) != 1 {
		r.logger.Error(nil, "expected exactly one web process", "processes", processList)
		return korifiv1alpha1.CFProcess{}, fmt.Errorf("expected exactly one web process, found %d", len(processList.Items))
	}

	return processList.Items[0], err
}

func (r *CFTaskReconciler) createOrPatchTaskWorkload(ctx context.Context, cfTask *korifiv1alpha1.CFTask, cfDroplet *korifiv1alpha1.CFBuild, webProcess korifiv1alpha1.CFProcess, env []corev1.EnvVar) (*korifiv1alpha1.TaskWorkload, error) {
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

		taskWorkload.Spec.Resources.Requests[corev1.ResourceMemory] = *resource.NewScaledQuantity(r.cfProcessDefaults.MemoryMB, resource.Mega)
		taskWorkload.Spec.Resources.Limits[corev1.ResourceMemory] = *resource.NewScaledQuantity(r.cfProcessDefaults.MemoryMB, resource.Mega)
		taskWorkload.Spec.Resources.Requests[corev1.ResourceEphemeralStorage] = *resource.NewScaledQuantity(r.cfProcessDefaults.DiskQuotaMB, resource.Mega)
		taskWorkload.Spec.Resources.Limits[corev1.ResourceEphemeralStorage] = *resource.NewScaledQuantity(r.cfProcessDefaults.DiskQuotaMB, resource.Mega)
		taskWorkload.Spec.Resources.Requests[corev1.ResourceCPU] = *resource.NewScaledQuantity(calculateDefaultCPURequestMillicores(webProcess.Spec.MemoryMB), resource.Milli)
		taskWorkload.Spec.Env = env

		if err := ctrl.SetControllerReference(cfTask, taskWorkload, r.scheme); err != nil {
			r.logger.Error(err, "failed to set owner ref")
			return err
		}

		return nil
	})
	if err != nil {
		r.logger.Info("error-creating-or-patching-task-workload", "error", err, "opResult", opResult)
		return nil, err
	}

	if opResult == controllerutil.OperationResultCreated {
		r.recorder.Eventf(cfTask, "Normal", "taskCreated", "Created task workload %s", taskWorkload.Name)
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

func (r *CFTaskReconciler) ensureInitialized(ctx context.Context, cfTask *korifiv1alpha1.CFTask, cfDroplet *korifiv1alpha1.CFBuild) error {
	if cfTask.Status.SequenceID == 0 {
		var err error
		cfTask.Status.SequenceID, err = r.seqIdGenerator.Generate()
		if err != nil {
			r.logger.Info("error-generating-sequence-id", "error", err)
			return err
		}

		cfTask.Status.MemoryMB = r.cfProcessDefaults.MemoryMB
		cfTask.Status.DiskQuotaMB = r.cfProcessDefaults.DiskQuotaMB
		cfTask.Status.DropletRef.Name = cfDroplet.Name
		meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.TaskInitializedConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "taskInitialized",
			Message: "taskInitialized",
		})

	}

	return nil
}

func (r *CFTaskReconciler) handleCancelation(ctx context.Context, cfTask *korifiv1alpha1.CFTask) error {
	taskWorkload := &korifiv1alpha1.TaskWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfTask.Name,
			Namespace: cfTask.Namespace,
		},
	}
	err := r.k8sClient.Delete(ctx, taskWorkload)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.logger.Info("error-deleting-task-workload", "error", err)
		return err
	}

	meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
		Type:   korifiv1alpha1.TaskCanceledConditionType,
		Status: metav1.ConditionTrue,
		Reason: TaskCanceledReason,
	})

	if !meta.IsStatusConditionTrue(cfTask.Status.Conditions, korifiv1alpha1.TaskSucceededConditionType) {
		meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
			Type:   korifiv1alpha1.TaskFailedConditionType,
			Status: metav1.ConditionTrue,
			Reason: TaskCanceledReason,
		})
	}

	return nil
}

func (r *CFTaskReconciler) updateStatusAndReturn(ctx context.Context, cfTask *korifiv1alpha1.CFTask, reconcileErr error) (ctrl.Result, error) {
	orig := &korifiv1alpha1.CFTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfTask.Name,
			Namespace: cfTask.Namespace,
		},
	}
	if statusErr := r.k8sClient.Status().Patch(ctx, cfTask, client.MergeFrom(orig)); statusErr != nil {
		r.logger.Error(statusErr, "unable to patch CFTask status")
		return ctrl.Result{}, statusErr
	}

	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	return ctrl.Result{RequeueAfter: r.computeRequeueAfter(cfTask)}, nil
}

func (r *CFTaskReconciler) computeRequeueAfter(cfTask *korifiv1alpha1.CFTask) time.Duration {
	completeTime, isCompleted := getCompletionTime(cfTask)
	if !isCompleted {
		return 0
	}

	return time.Until(completeTime.Add(r.taskTTLDuration))
}

func (r *CFTaskReconciler) alreadyExpired(cfTask *korifiv1alpha1.CFTask) bool {
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
