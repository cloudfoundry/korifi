/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	workloadContainerName = "workload"
	ServiceAccountName    = "korifi-task"
)

//counterfeiter:generate -o fake -fake-name TaskStatusGetter . TaskStatusGetter

type TaskStatusGetter interface {
	GetStatusConditions(ctx context.Context, job *batchv1.Job) ([]metav1.Condition, error)
}

// TaskWorkloadReconciler reconciles a TaskWorkload object
type TaskWorkloadReconciler struct {
	k8sClient    client.Client
	logger       logr.Logger
	scheme       *runtime.Scheme
	statusGetter TaskStatusGetter
	jobTTL       time.Duration
}

func NewTaskWorkloadReconciler(
	logger logr.Logger,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	statusGetter TaskStatusGetter,
	jobTTL time.Duration,
) *k8s.PatchingReconciler[korifiv1alpha1.TaskWorkload, *korifiv1alpha1.TaskWorkload] {
	taskReconciler := TaskWorkloadReconciler{
		k8sClient:    k8sClient,
		logger:       logger,
		scheme:       scheme,
		statusGetter: statusGetter,
		jobTTL:       jobTTL,
	}

	return k8s.NewPatchingReconciler[korifiv1alpha1.TaskWorkload, *korifiv1alpha1.TaskWorkload](logger, k8sClient, &taskReconciler)
}

func (r *TaskWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.TaskWorkload{}).
		Owns(&batchv1.Job{})
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/status,verbs=get;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *TaskWorkloadReconciler) ReconcileResource(ctx context.Context, taskWorkload *korifiv1alpha1.TaskWorkload) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	taskWorkload.Status.ObservedGeneration = taskWorkload.Generation
	log.V(1).Info("set observed generation", "generation", taskWorkload.Status.ObservedGeneration)

	job, err := r.getOrCreateJob(ctx, log, taskWorkload)
	if err != nil {
		return ctrl.Result{}, err
	}

	if job == nil {
		return ctrl.Result{}, nil
	}

	if err = r.updateTaskWorkloadStatus(ctx, taskWorkload, job); err != nil {
		log.Info("failed to update task workload status", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r TaskWorkloadReconciler) getOrCreateJob(ctx context.Context, logger logr.Logger, taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job := &batchv1.Job{}

	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(taskWorkload), job)
	if err == nil {
		return job, nil
	}

	if k8serrors.IsNotFound(err) {
		if meta.IsStatusConditionTrue(taskWorkload.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType) {
			return nil, nil
		}

		return r.createJob(ctx, logger, taskWorkload)
	}

	logger.Info("getting job failed", "reason", err)
	return nil, err
}

func (r TaskWorkloadReconciler) createJob(ctx context.Context, logger logr.Logger, taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job, err := r.workloadToJob(taskWorkload)
	if err != nil {
		logger.Info("failed to convert task workload to job", "reason", err)
		return nil, err
	}

	err = r.k8sClient.Create(ctx, job)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logger.V(1).Info("job for TaskWorkload already exists")
		} else {
			logger.Info("failed to create job for task workload", "reason", err)
		}
		return nil, err
	}

	return job, nil
}

func (r *TaskWorkloadReconciler) workloadToJob(taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskWorkload.Name,
			Namespace: taskWorkload.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            tools.PtrTo(int32(0)),
			Parallelism:             tools.PtrTo(int32(1)),
			Completions:             tools.PtrTo(int32(1)),
			TTLSecondsAfterFinished: tools.PtrTo(int32(r.jobTTL.Seconds())),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: tools.PtrTo(true),
					},
					AutomountServiceAccountToken: tools.PtrTo(false),
					ImagePullSecrets:             taskWorkload.Spec.ImagePullSecrets,
					Containers: []corev1.Container{{
						Name:      workloadContainerName,
						Image:     taskWorkload.Spec.Image,
						Command:   taskWorkload.Spec.Command,
						Resources: taskWorkload.Spec.Resources,
						Env:       taskWorkload.Spec.Env,
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							AllowPrivilegeEscalation: tools.PtrTo(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					}},
					ServiceAccountName: ServiceAccountName,
				},
			},
		},
	}

	err := controllerutil.SetControllerReference(taskWorkload, job, r.scheme)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (r *TaskWorkloadReconciler) updateTaskWorkloadStatus(ctx context.Context, taskWorkload *korifiv1alpha1.TaskWorkload, job *batchv1.Job) error {
	conditions, err := r.statusGetter.GetStatusConditions(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to get status conditions for job %s:%s: %w", job.Namespace, job.Name, err)
	}

	for _, condition := range conditions {
		condition.ObservedGeneration = taskWorkload.Generation
		meta.SetStatusCondition(&taskWorkload.Status.Conditions, condition)
	}

	return nil
}
