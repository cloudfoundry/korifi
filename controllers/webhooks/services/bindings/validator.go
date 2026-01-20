package bindings

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	validation "code.cloudfoundry.org/korifi/controllers/webhooks/validation"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	ServiceBindingEntityType = "servicebinding"
	ServiceBindingErrorType  = "ServiceBindingValidationError"
)

// log is for logging in this package.
var cfservicebindinglog = logf.Log.WithName("cfservicebinding-validator")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfservicebinding,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=create;update;delete,versions=v1alpha1,name=vcfservicebinding.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

func (v *CFServiceBindingValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &korifiv1alpha1.CFServiceBinding{}).
		WithValidator(v).
		Complete()
}

type CFServiceBindingValidator struct {
	duplicateValidator webhooks.NameValidator
}

var _ admission.Validator[*korifiv1alpha1.CFServiceBinding] = &CFServiceBindingValidator{}

func NewCFServiceBindingValidator(duplicateValidator webhooks.NameValidator) *CFServiceBindingValidator {
	return &CFServiceBindingValidator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *CFServiceBindingValidator) ValidateCreate(ctx context.Context, serviceBinding *korifiv1alpha1.CFServiceBinding) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateCreate(ctx, cfservicebindinglog, serviceBinding.Namespace, serviceBinding)
}

func (v *CFServiceBindingValidator) ValidateUpdate(ctx context.Context, oldServiceBinding, serviceBinding *korifiv1alpha1.CFServiceBinding) (admission.Warnings, error) {
	if !serviceBinding.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	if oldServiceBinding.Spec.AppRef.Name != serviceBinding.Spec.AppRef.Name {
		return nil, validation.ValidationError{Type: ServiceBindingErrorType, Message: "AppRef.Name is immutable"}
	}

	if oldServiceBinding.Spec.Service.Name != serviceBinding.Spec.Service.Name {
		return nil, validation.ValidationError{Type: ServiceBindingErrorType, Message: "Service.Name is immutable"}
	}

	if oldServiceBinding.Spec.Service.Namespace != serviceBinding.Spec.Service.Namespace {
		return nil, validation.ValidationError{Type: ServiceBindingErrorType, Message: "Service.Namespace is immutable"}
	}

	return nil, nil
}

func (v *CFServiceBindingValidator) ValidateDelete(ctx context.Context, serviceBinding *korifiv1alpha1.CFServiceBinding) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateDelete(ctx, cfservicebindinglog, serviceBinding.Namespace, serviceBinding)
}
