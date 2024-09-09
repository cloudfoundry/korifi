package instances

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	validationwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	ServiceInstanceEntityType = "serviceinstance"
)

var cfserviceinstancelog = logf.Log.WithName("cfserviceinstance-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfserviceinstance,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=create;update;delete,versions=v1alpha1,name=vcfserviceinstance.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type Validator struct {
	duplicateValidator webhooks.NameValidator
}

var _ webhook.CustomValidator = &Validator{}

func NewValidator(duplicateValidator webhooks.NameValidator) *Validator {
	return &Validator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		WithValidator(v).
		Complete()
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateCreate(ctx, cfserviceinstancelog, serviceInstance.Namespace, serviceInstance)
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	if !serviceInstance.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	oldServiceInstance, ok := oldObj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", oldObj))
	}

	if serviceInstance.Spec.Type != oldServiceInstance.Spec.Type {
		return nil, validationwebhook.ValidationError{
			Type:    validationwebhook.ImmutableFieldErrorType,
			Message: fmt.Sprintf(validationwebhook.ImmutableFieldErrorMessageTemplate, "CFServiceInstance.Spec.Type"),
		}.ExportJSONError()
	}
	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfserviceinstancelog, serviceInstance.Namespace, oldServiceInstance, serviceInstance)
}

func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateDelete(ctx, cfserviceinstancelog, serviceInstance.Namespace, serviceInstance)
}
