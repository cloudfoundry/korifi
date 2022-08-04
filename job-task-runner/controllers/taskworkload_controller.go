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

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// TaskWorkloadReconciler reconciles a TaskWorkload object
type TaskWorkloadReconciler struct {
	k8sClient client.Client
	logger    logr.Logger
	scheme    *runtime.Scheme
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads,verbs=get;list;watch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=create

func NewTaskWorkloadReconciler(logger logr.Logger, k8sClient client.Client, scheme *runtime.Scheme) *TaskWorkloadReconciler {
	return &TaskWorkloadReconciler{
		k8sClient: k8sClient,
		logger:    logger,
		scheme:    scheme,
	}
}

func (r *TaskWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	taskWorkload := &korifiv1alpha1.TaskWorkload{}
	err := r.k8sClient.Get(ctx, req.NamespacedName, taskWorkload)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.logger.Info("TaskWorkload not found", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}

		r.logger.Error(err, "failed to get TaskWorkload", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	job, err := r.workloadToJob(taskWorkload)
	if err != nil {
		r.logger.Error(err, "failed to convert task workload to job", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	err = r.k8sClient.Create(ctx, job)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			r.logger.Info("job for TaskWorkload already exists", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}

		r.logger.Error(err, "failed to create job for task workload", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.TaskWorkload{}).
		Complete(r)
}

func (r *TaskWorkloadReconciler) workloadToJob(taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskWorkload.Name,
			Namespace: taskWorkload.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "workload",
						Image:   taskWorkload.Spec.Image,
						Command: taskWorkload.Spec.Command,
						Env:     taskWorkload.Spec.Env,
					}},
				},
			},
		},
	}

	err := controllerutil.SetOwnerReference(taskWorkload, job, r.scheme)
	if err != nil {
		return nil, err
	}

	return job, nil
}
