package services

import (
	"context"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name NameValidator . NameValidator

type NameValidator interface {
	ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName string) error
	ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, newName string) error
	ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) error
}

const (
	ServiceInstanceEntityType = "serviceinstance"

	ServiceInstanceDecodingErrorType      = "ServiceInstanceDecodingError"
	DuplicateServiceInstanceNameErrorType = "DuplicateServiceInstanceNameError"
	// Note: the cf cli expects the specific text 'The service instance name is taken'
	duplicateServiceInstanceNameErrorMessage = "The service instance name is taken: %s"
)

var cfserviceinstancelog = logf.Log.WithName("cfserviceinstance-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfserviceinstance,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfserviceinstances,verbs=create;update;delete,versions=v1alpha1,name=vcfserviceinstance.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFServiceInstanceValidator struct {
	duplicateValidator NameValidator
}

var _ webhook.CustomValidator = &CFServiceInstanceValidator{}

func NewCFServiceInstanceValidator(duplicateValidator NameValidator) *CFServiceInstanceValidator {
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

func (v *CFServiceInstanceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfserviceinstancelog, serviceInstance.Namespace, serviceInstance.Spec.DisplayName)
	if validationErr != nil {
		if errors.Is(validationErr, webhooks.ErrorDuplicateName) {
			errorMessage := fmt.Sprintf(duplicateServiceInstanceNameErrorMessage, serviceInstance.Spec.DisplayName)
			return errors.New(webhooks.ValidationError{
				Type:    DuplicateServiceInstanceNameErrorType,
				Message: errorMessage,
			}.Marshal())
		}

		return errors.New(webhooks.AdmissionUnknownErrorReason())
	}

	return nil
}

func (v *CFServiceInstanceValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) error {
	oldServiceInstance, ok := oldObj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", oldObj))
	}

	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateUpdate(ctx, cfserviceinstancelog, serviceInstance.Namespace, oldServiceInstance.Spec.DisplayName, serviceInstance.Spec.DisplayName)
	if validationErr != nil {
		if errors.Is(validationErr, webhooks.ErrorDuplicateName) {
			errorMessage := fmt.Sprintf(duplicateServiceInstanceNameErrorMessage, serviceInstance.Spec.DisplayName)
			return errors.New(webhooks.ValidationError{
				Type:    DuplicateServiceInstanceNameErrorType,
				Message: errorMessage,
			}.Marshal())
		}

		return errors.New(webhooks.AdmissionUnknownErrorReason())
	}
	return nil
}

func (v *CFServiceInstanceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	serviceInstance, ok := obj.(*korifiv1alpha1.CFServiceInstance)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceInstance but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfserviceinstancelog, serviceInstance.Namespace, serviceInstance.Spec.DisplayName)
	if validationErr != nil {
		return errors.New(webhooks.AdmissionUnknownErrorReason())
	}

	return nil
}
