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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	ServiceInstanceEntityType = "serviceinstance"
	// Note: the cf cli expects the specific text 'The service instance name is taken'
	duplicateServiceInstanceNameErrorMessage = "The service instance name is taken: %s"
)

var cfserviceinstancelog = logf.Log.WithName("cfserviceinstance-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfserviceinstance,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=create;update;delete,versions=v1alpha1,name=vcfserviceinstance.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFServiceInstanceValidator struct {
	duplicateValidator webhooks.NameValidator
}

var _ webhook.CustomValidator = &CFServiceInstanceValidator{}

func NewCFServiceInstanceValidator(duplicateValidator webhooks.NameValidator) *CFServiceInstanceValidator {
	return &CFServiceInstanceValidator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *CFServiceInstanceValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceInstance{}).
		WithValidator(v).
		Complete()
}

func (v *CFServiceInstanceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateServiceInstanceNameErrorMessage, serviceInstance.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfserviceinstancelog, serviceInstance.Namespace, serviceInstance.Spec.DisplayName, duplicateErrorMessage)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFServiceInstanceValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
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

	duplicateErrorMessage := fmt.Sprintf(duplicateServiceInstanceNameErrorMessage, serviceInstance.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateUpdate(ctx, cfserviceinstancelog, serviceInstance.Namespace, oldServiceInstance.Spec.DisplayName, serviceInstance.Spec.DisplayName, duplicateErrorMessage)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFServiceInstanceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfserviceinstancelog, serviceInstance.Namespace, serviceInstance.Spec.DisplayName)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}
