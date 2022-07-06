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
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	MissingRequredFieldErrorType    = "MissingRequiredFieldError"
	CancelationNotPossibleErrorType = "CancelaionNotPossibleError"
)

// log is for logging in this package.
var cftasklog = logf.Log.WithName("cftask-resource")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cftask,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cftasks,verbs=create;update,versions=v1alpha1,name=vcftask.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFTaskValidator struct{}

var _ webhook.CustomValidator = &CFTaskValidator{}

func NewCFTaskValidator() *CFTaskValidator {
	return &CFTaskValidator{}
}

func (v *CFTaskValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.CFTask{}).
		WithValidator(v).
		Complete()
}

var _ webhook.CustomValidator = &CFTaskValidator{}

func (v *CFTaskValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	task, ok := obj.(*v1alpha1.CFTask)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFTask but got a %T", obj))
	}

	cftasklog.Info("validate task creation", "namespace", task.Namespace, "name", task.Name)

	if len(task.Spec.Command) == 0 {
		return webhooks.ValidationError{
			Type:    MissingRequredFieldErrorType,
			Message: fmt.Sprintf("task %s:%s is missing required field 'Spec.Command'", task.Namespace, task.Name),
		}.ExportJSONError()
	}

	if task.Spec.AppRef.Name == "" {
		return webhooks.ValidationError{
			Type:    MissingRequredFieldErrorType,
			Message: fmt.Sprintf("task %s:%s is missing required field 'Spec.AppRef.Name'", task.Namespace, task.Name),
		}.ExportJSONError()
	}

	return nil
}

func (v *CFTaskValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, obj runtime.Object) error {
	newTask, ok := obj.(*v1alpha1.CFTask)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFTask but got a %T", obj))
	}

	cftasklog.Info("validate task update", "namespace", newTask.Namespace, "name", newTask.Name)

	oldTask, ok := oldObj.(*v1alpha1.CFTask)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFTask but got a %T", oldObj))
	}

	if !newTask.Spec.Canceled || oldTask.Spec.Canceled {
		return nil
	}

	taskSucceeded := meta.IsStatusConditionTrue(newTask.Status.Conditions, v1alpha1.TaskSucceededConditionType)
	taskFailed := meta.IsStatusConditionTrue(newTask.Status.Conditions, v1alpha1.TaskFailedConditionType)

	if taskSucceeded || taskFailed {
		return webhooks.ValidationError{
			Type:    CancelationNotPossibleErrorType,
			Message: fmt.Sprintf("task %s:%s cannot be canceled, because it has already terminated", newTask.Namespace, newTask.Name),
		}.ExportJSONError()
	}

	return nil
}

func (v *CFTaskValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}
