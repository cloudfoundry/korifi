package services

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	ServiceBindingEntityType            = "servicebinding"
	ServiceBindingErrorType             = "ServiceBindingValidationError"
	duplicateServiceBindingErrorMessage = "Service binding already exists: App: %s Service Instance: %s"
)

// log is for logging in this package.
var cfservicebindinglog = logf.Log.WithName("cfservicebinding-validator")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfservicebinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfservicebindings,verbs=create;update;delete,versions=v1alpha1,name=vcfservicebinding.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

func (v *CFServiceBindingValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBinding{}).
		WithValidator(v).
		Complete()
}

type CFServiceBindingValidator struct {
	duplicateValidator webhooks.NameValidator
}

var _ webhook.CustomValidator = &CFServiceBindingValidator{}

func NewCFServiceBindingValidator(duplicateValidator webhooks.NameValidator) *CFServiceBindingValidator {
	return &CFServiceBindingValidator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *CFServiceBindingValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	serviceBinding, ok := obj.(*korifiv1alpha1.CFServiceBinding)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBinding but got a %T", obj))
	}

	lockName := generateServiceBindingLock(serviceBinding)

	duplicateErrorMessage := fmt.Sprintf(duplicateServiceBindingErrorMessage, serviceBinding.Spec.AppRef.Name, serviceBinding.Spec.Service.Name)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfservicebindinglog, serviceBinding.Namespace, lockName, duplicateErrorMessage)
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func (v *CFServiceBindingValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) error {
	serviceBinding, ok := obj.(*korifiv1alpha1.CFServiceBinding)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBinding but got a %T", obj))
	}

	if !serviceBinding.GetDeletionTimestamp().IsZero() {
		return nil
	}

	oldServiceBinding, ok := oldObj.(*korifiv1alpha1.CFServiceBinding)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBinding but got a %T", oldObj))
	}

	if oldServiceBinding.Spec.AppRef.Name != serviceBinding.Spec.AppRef.Name {
		return webhooks.ValidationError{Type: ServiceBindingErrorType, Message: "AppRef.Name is immutable"}
	}

	if oldServiceBinding.Spec.Service.Name != serviceBinding.Spec.Service.Name {
		return webhooks.ValidationError{Type: ServiceBindingErrorType, Message: "Service.Name is immutable"}
	}

	if oldServiceBinding.Spec.Service.Namespace != serviceBinding.Spec.Service.Namespace {
		return webhooks.ValidationError{Type: ServiceBindingErrorType, Message: "Service.Namespace is immutable"}
	}

	return nil
}

func (v *CFServiceBindingValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	serviceBinding, ok := obj.(*korifiv1alpha1.CFServiceBinding)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBinding but got a %T", obj))
	}

	lockName := generateServiceBindingLock(serviceBinding)

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfservicebindinglog, serviceBinding.Namespace, lockName)
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func generateServiceBindingLock(serviceBinding *korifiv1alpha1.CFServiceBinding) string {
	return fmt.Sprintf("sb::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Spec.Service.Namespace, serviceBinding.Spec.Service.Name)
}
