package workloads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	CFSpaceEntityType = "cfspace"
	// Note: the cf cli expects the specific text `Name must be unique per organization` in the error and ignores the error if it matches it.
	duplicateSpaceNameErrorMessage = "Space '%s' already exists. Name must be unique per organization."
	SpacePlacementErrorType        = "SpacePlacementError"
)

var spaceLogger = logf.Log.WithName("cfspace-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfspaces,verbs=create;update;delete,versions=v1alpha1,name=vcfspace.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFSpaceValidator struct {
	duplicateValidator NameValidator
	placementValidator PlacementValidator
}

var _ webhook.CustomValidator = &CFSpaceValidator{}

func NewCFSpaceValidator(duplicateSpaceValidator NameValidator, placementValidator PlacementValidator) *CFSpaceValidator {
	return &CFSpaceValidator{
		duplicateValidator: duplicateSpaceValidator,
		placementValidator: placementValidator,
	}
}

func (v *CFSpaceValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFSpace{}).
		WithValidator(v).
		Complete()
}

func (v *CFSpaceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	space, ok := obj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateSpaceNameErrorMessage, space.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, spaceLogger, space.Namespace, strings.ToLower(space.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return errors.New(validationErr.Marshal())
	}

	err := v.placementValidator.ValidateSpaceCreate(*space)
	if err != nil {
		return errors.New(webhooks.ValidationError{
			Type:    SpacePlacementErrorType,
			Message: err.Error(),
		}.Marshal())
	}

	return nil
}

func (v *CFSpaceValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) error {
	oldSpace, ok := oldObj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	space, ok := obj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateSpaceNameErrorMessage, space.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateUpdate(ctx, spaceLogger, oldSpace.Namespace, strings.ToLower(oldSpace.Spec.DisplayName), strings.ToLower(space.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return errors.New(validationErr.Marshal())
	}

	return nil
}

func (v *CFSpaceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	space, ok := obj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, spaceLogger, space.Namespace, space.Spec.DisplayName)
	if validationErr != nil {
		return errors.New(validationErr.Marshal())
	}

	return nil
}
