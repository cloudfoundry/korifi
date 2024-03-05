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
	ServiceBrokerEntityType = "servicebroker"
	// ServiceBrokerErrorType  = "ServiceBrokerValidationError"
)

// log is for logging in this package.
var cfservicebrokerlog = logf.Log.WithName("cfservicebroker-validator")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfservicebroker,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cfservicebrokers,verbs=create;update;delete,versions=v1alpha1,name=vcfservicebroker.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

func (v *CFServiceBrokerValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFServiceBroker{}).
		WithValidator(v).
		Complete()
}

type CFServiceBrokerValidator struct {
	duplicateValidator webhooks.NameValidator
}

var _ webhook.CustomValidator = &CFServiceBrokerValidator{}

func NewCFServiceBrokerValidator(duplicateValidator webhooks.NameValidator) *CFServiceBrokerValidator {
	return &CFServiceBrokerValidator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *CFServiceBrokerValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceBroker, ok := obj.(*korifiv1alpha1.CFServiceBroker)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBroker but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateCreate(ctx, cfservicebrokerlog, serviceBroker.Namespace, serviceBroker)
}

func (v *CFServiceBrokerValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	serviceBroker, ok := obj.(*korifiv1alpha1.CFServiceBroker)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBroker but got a %T", obj))
	}

	if !serviceBroker.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	oldServiceBroker, ok := oldObj.(*korifiv1alpha1.CFServiceBroker)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBroker but got a %T", oldObj))
	}

	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfservicebrokerlog, serviceBroker.Namespace, oldServiceBroker, serviceBroker)
}

func (v *CFServiceBrokerValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceBroker, ok := obj.(*korifiv1alpha1.CFServiceBroker)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFServiceBroker but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateDelete(ctx, cfservicebrokerlog, serviceBroker.Namespace, serviceBroker)
}
