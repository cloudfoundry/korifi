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

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

const workloadContainerName = "workload"

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
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads,verbs=get;list;watch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func NewTaskWorkloadReconciler(logger logr.Logger, k8sClient client.Client, scheme *runtime.Scheme, statusGetter TaskStatusGetter) *TaskWorkloadReconciler {
	return &TaskWorkloadReconciler{
		k8sClient:    k8sClient,
		logger:       logger,
		scheme:       scheme,
		statusGetter: statusGetter,
	}
}

func (r *TaskWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("namespace", req.Namespace, "name", req.Name)

	taskWorkload := &korifiv1alpha1.TaskWorkload{}
	err := r.k8sClient.Get(ctx, req.NamespacedName, taskWorkload)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("TaskWorkload not found")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "failed to get TaskWorkload")
		return ctrl.Result{}, err
	}

	job, err := r.getOrCreateJob(ctx, logger, taskWorkload)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateTaskWorkloadStatus(ctx, taskWorkload, job); err != nil {
		logger.Error(err, "failed to update task workload status")
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

	if !k8serrors.IsNotFound(err) {
		logger.Error(err, "getting job failed")
		return nil, err
	}

	return r.createJob(ctx, logger, taskWorkload)
}

func (r TaskWorkloadReconciler) createJob(ctx context.Context, logger logr.Logger, taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job, err := r.workloadToJob(taskWorkload)
	if err != nil {
		logger.Error(err, "failed to convert task workload to job")
		return nil, err
	}

	err = r.k8sClient.Create(ctx, job)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logger.Info("job for TaskWorkload already exists")
		} else {
			logger.Error(err, "failed to create job for task workload")
		}
		return nil, err
	}

	return job, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.TaskWorkload{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *TaskWorkloadReconciler) workloadToJob(taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskWorkload.Name,
			Namespace: taskWorkload.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: tools.PtrTo(int32(0)),
			Parallelism:  tools.PtrTo(int32(1)),
			Completions:  tools.PtrTo(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: tools.PtrTo(true),
					},
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
	originalTaskWorkload := taskWorkload.DeepCopy()

	conditions, err := r.statusGetter.GetStatusConditions(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to get status conditions for job %s:%s: %w", job.Namespace, job.Name, err)
	}

	for _, condition := range conditions {
		meta.SetStatusCondition(&taskWorkload.Status.Conditions, condition)
	}

	return r.k8sClient.Status().Patch(ctx, taskWorkload, client.MergeFrom(originalTaskWorkload))
}
