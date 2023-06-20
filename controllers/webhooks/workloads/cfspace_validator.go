package workloads

import (
	"context"
	"errors"
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
	CFSpaceEntityType = "cfspace"
)

var spaceLogger = logf.Log.WithName("cfspace-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfspace,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cfspaces,verbs=create;update;delete,versions=v1alpha1,name=vcfspace.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFSpaceValidator struct {
	duplicateValidator webhooks.NameValidator
	placementValidator webhooks.NamespaceValidator
}

var _ webhook.CustomValidator = &CFSpaceValidator{}

func NewCFSpaceValidator(duplicateSpaceValidator webhooks.NameValidator, placementValidator webhooks.NamespaceValidator) *CFSpaceValidator {
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

func (v *CFSpaceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	space, ok := obj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	if len(space.Name) > maxLabelLength {
		return nil, errors.New("space name cannot be longer than 63 chars")
	}

	validationErr := v.duplicateValidator.ValidateCreate(ctx, spaceLogger, space.Namespace, space)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	err := v.placementValidator.ValidateSpaceCreate(*space)
	if err != nil {
		return nil, err.ExportJSONError()
	}

	return nil, nil
}

func (v *CFSpaceValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	space, ok := obj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	if !space.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	oldSpace, ok := oldObj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateUpdate(ctx, spaceLogger, oldSpace.Namespace, oldSpace, space)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFSpaceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	space, ok := obj.(*korifiv1alpha1.CFSpace)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSpace but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, spaceLogger, space.Namespace, space)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}
