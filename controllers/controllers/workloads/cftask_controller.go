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

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

//counterfeiter:generate -o fake -fake-name SeqIdGenerator . SeqIdGenerator
type SeqIdGenerator interface {
	Generate() (int64, error)
}

// CFTaskReconciler reconciles a CFTask object
type CFTaskReconciler struct {
	k8sClient      client.Client
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	logger         logr.Logger
	seqIdGenerator SeqIdGenerator
}

func NewCFTaskReconciler(client client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, logger logr.Logger, seqIdGenerator SeqIdGenerator) *CFTaskReconciler {
	return &CFTaskReconciler{
		k8sClient:      client,
		scheme:         scheme,
		recorder:       recorder,
		logger:         logger,
		seqIdGenerator: seqIdGenerator,
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cftasks/finalizers,verbs=update
//+kubebuilder:rbac:groups=eirini.cloudfoundry.org,resources=tasks,verbs=create
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *CFTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	cfTask := new(korifiv1alpha1.CFTask)
	err := r.k8sClient.Get(ctx, req.NamespacedName, cfTask)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.logger.Info("error-getting-cftask", "error", err)
		return ctrl.Result{}, err
	}

	if cfTask.Status.SequenceID == 0 {
		cfTaskCopy := cfTask.DeepCopy()
		cfTaskCopy.Status.SequenceID, err = r.seqIdGenerator.Generate()
		if err != nil {
			r.logger.Info("error-generating-sequence-id", "error", err)
			return ctrl.Result{}, err
		}

		err = r.k8sClient.Status().Patch(ctx, cfTaskCopy, client.MergeFrom(cfTask))
		if err != nil {
			r.logger.Info("error-updating-status", "error", err)
			return ctrl.Result{}, err
		}
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      cfTask.Spec.AppRef.Name,
	}, cfApp)
	if err != nil {
		r.logger.Info("error-getting-cfapp", "error", err)
		if k8serrors.IsNotFound(err) {
			r.recorder.Eventf(cfTask, "Warning", "appNotFound", "Did not find app with name %s in namespace %s", cfTask.Spec.AppRef.Name, req.Namespace)
		}
		return ctrl.Result{}, err
	}

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		r.recorder.Eventf(cfTask, "Warning", "appCurrentDropletRefNotSet", "App %s does not have a current droplet", cfTask.Spec.AppRef.Name)
		return ctrl.Result{}, errors.New("app droplet ref not set")
	}

	cfDroplet := new(korifiv1alpha1.CFBuild)
	err = r.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      cfApp.Spec.CurrentDropletRef.Name,
	}, cfDroplet)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.recorder.Eventf(cfTask, "Warning", "appCurrentDropletNotFound", "Current droplet %s for app %s does not exist", cfApp.Spec.CurrentDropletRef.Name, cfTask.Spec.AppRef.Name)
		}
		r.logger.Info("error-getting-cfdroplet", "error", err)
		return ctrl.Result{}, err
	}

	if cfDroplet.Status.Droplet == nil {
		r.recorder.Eventf(cfTask, "Warning", "dropletBuildStatusNotSet", "Current droplet %s from app %s does not have a droplet image", cfApp.Spec.CurrentDropletRef.Name, cfTask.Spec.AppRef.Name)
		return ctrl.Result{}, errors.New("droplet build status not set")
	}

	eiriniTask := &eiriniv1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfTask.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFTaskGUIDLabelKey: cfTask.Name,
			},
		},
		Spec: eiriniv1.TaskSpec{
			GUID:    cfTask.Name,
			Command: cfTask.Spec.Command,
			Image:   cfDroplet.Status.Droplet.Registry.Image,
		},
	}

	err = r.k8sClient.Create(ctx, eiriniTask)
	if err != nil {
		r.logger.Info("error-creating-eirini-task", "error", err)
		if k8serrors.IsAlreadyExists(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	r.recorder.Eventf(cfTask, "Normal", "taskCreated", "Created eirini task %s", eiriniTask.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFTask{}).
		Complete(r)
}
