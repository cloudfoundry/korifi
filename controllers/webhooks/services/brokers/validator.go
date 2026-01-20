package brokers

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	ServiceBrokerEntityType = "servicebroker"
)

var cfservicebrokerlog = logf.Log.WithName("cfservicebroker-validator")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfservicebroker,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cfservicebrokers,verbs=create;update;delete,versions=v1alpha1,name=vcfservicebroker.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &korifiv1alpha1.CFServiceBroker{}).
		WithValidator(v).
		Complete()
}

type Validator struct {
	duplicateValidator webhooks.NameValidator
}

var _ admission.Validator[*korifiv1alpha1.CFServiceBroker] = &Validator{}

func NewValidator(duplicateValidator webhooks.NameValidator) *Validator {
	return &Validator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, serviceBroker *korifiv1alpha1.CFServiceBroker) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateCreate(ctx, cfservicebrokerlog, serviceBroker.Namespace, serviceBroker)
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldServiceBroker, serviceBroker *korifiv1alpha1.CFServiceBroker) (admission.Warnings, error) {
	if !serviceBroker.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfservicebrokerlog, serviceBroker.Namespace, oldServiceBroker, serviceBroker)
}

func (v *Validator) ValidateDelete(ctx context.Context, serviceBroker *korifiv1alpha1.CFServiceBroker) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateDelete(ctx, cfservicebrokerlog, serviceBroker.Namespace, serviceBroker)
}
