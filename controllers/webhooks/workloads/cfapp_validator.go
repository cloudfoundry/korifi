package workloads

import (
	"context"
	"fmt"
	"strings"

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
	AppEntityType                = "app"
	AppDecodingErrorType         = "AppDecodingError"
	duplicateAppNameErrorMessage = "App with the name '%s' already exists."
)

var cfapplog = logf.Log.WithName("cfapp-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=create;update;delete,versions=v1alpha1,name=vcfapp.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFAppValidator struct {
	duplicateValidator webhooks.NameValidator
}

var _ webhook.CustomValidator = &CFAppValidator{}

func NewCFAppValidator(duplicateValidator webhooks.NameValidator) *CFAppValidator {
	return &CFAppValidator{
		duplicateValidator: duplicateValidator,
	}
}

func (v *CFAppValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFApp{}).
		WithValidator(v).
		Complete()
}

func (v *CFAppValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	app, ok := obj.(*korifiv1alpha1.CFApp)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateAppNameErrorMessage, app.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfapplog, app.Namespace, strings.ToLower(app.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFAppValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	app, ok := obj.(*korifiv1alpha1.CFApp)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", obj))
	}

	if !app.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	oldApp, ok := oldObj.(*korifiv1alpha1.CFApp)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", oldObj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateAppNameErrorMessage, app.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateUpdate(ctx, cfapplog, app.Namespace, strings.ToLower(oldApp.Spec.DisplayName), strings.ToLower(app.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFAppValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	app, ok := obj.(*korifiv1alpha1.CFApp)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfapplog, app.Namespace, strings.ToLower(app.Spec.DisplayName))
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}
