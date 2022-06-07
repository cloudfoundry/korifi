package workloads

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	ValidateCreate(ctx context.Context, logger logr.Logger, namespace, newName, duplicateNameError string) *webhooks.ValidationError
	ValidateUpdate(ctx context.Context, logger logr.Logger, namespace, oldName, duplicateNameError, newName string) *webhooks.ValidationError
	ValidateDelete(ctx context.Context, logger logr.Logger, namespace, oldName string) *webhooks.ValidationError
}

const (
	AppEntityType                = "app"
	AppDecodingErrorType         = "AppDecodingError"
	duplicateAppNameErrorMessage = "App with the name '%s' already exists."
)

var cfapplog = logf.Log.WithName("cfapp-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps,verbs=create;update;delete,versions=v1alpha1,name=vcfapp.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFAppValidator struct {
	duplicateValidator NameValidator
}

var _ webhook.CustomValidator = &CFAppValidator{}

func NewCFAppValidator(duplicateValidator NameValidator) *CFAppValidator {
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

func (v *CFAppValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	app, ok := obj.(*korifiv1alpha1.CFApp)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateAppNameErrorMessage, app.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfapplog, app.Namespace, strings.ToLower(app.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return errors.New(validationErr.Marshal())
	}

	return nil
}

func (v *CFAppValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) error {
	oldApp, ok := oldObj.(*korifiv1alpha1.CFApp)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", oldObj))
	}

	app, ok := obj.(*korifiv1alpha1.CFApp)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateAppNameErrorMessage, app.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateUpdate(ctx, cfapplog, app.Namespace, strings.ToLower(oldApp.Spec.DisplayName), strings.ToLower(app.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return errors.New(validationErr.Marshal())
	}

	return nil
}

func (v *CFAppValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	app, ok := obj.(*korifiv1alpha1.CFApp)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFApp but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfapplog, app.Namespace, strings.ToLower(app.Spec.DisplayName))
	if validationErr != nil {
		return errors.New(validationErr.Marshal())
	}

	return nil
}
