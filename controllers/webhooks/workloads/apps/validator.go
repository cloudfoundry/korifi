package apps

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	AppEntityType        = "app"
	AppDecodingErrorType = "AppDecodingError"
)

var cfapplog = logf.Log.WithName("cfapp-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfapp,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=create;update;delete,versions=v1alpha1,name=vcfapp.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type Validator struct {
	duplicateValidator webhooks.NameValidator
}

var _ admission.Validator[*korifiv1alpha1.CFApp] = &Validator{}

func NewValidator(duplicateValidator webhooks.NameValidator) *Validator {
	return &Validator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &korifiv1alpha1.CFApp{}).
		WithValidator(v).
		Complete()
}

func (v *Validator) ValidateCreate(ctx context.Context, app *korifiv1alpha1.CFApp) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateCreate(ctx, cfapplog, app.Namespace, app)
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldApp, app *korifiv1alpha1.CFApp) (admission.Warnings, error) {
	if !app.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	if app.Spec.Lifecycle.Type != oldApp.Spec.Lifecycle.Type {
		return nil, validation.ValidationError{
			Type:    "ImmutableFieldError",
			Message: fmt.Sprintf("Lifecycle type cannot be changed from %s to %s", oldApp.Spec.Lifecycle.Type, app.Spec.Lifecycle.Type),
		}.ExportJSONError()
	}

	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfapplog, app.Namespace, oldApp, app)
}

func (v *Validator) ValidateDelete(ctx context.Context, app *korifiv1alpha1.CFApp) (admission.Warnings, error) {
	return nil, v.duplicateValidator.ValidateDelete(ctx, cfapplog, app.Namespace, app)
}
