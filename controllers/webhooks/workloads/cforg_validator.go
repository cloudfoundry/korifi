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
	CFOrgEntityType      = "cforg"
	OrgDecodingErrorType = "OrgDecodingError"
	maxLabelLength       = 63
)

var cfOrgLog = logf.Log.WithName("cforg-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cforg,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=korifi.cloudfoundry.org,resources=cforgs,verbs=create;update;delete,versions=v1alpha1,name=vcforg.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFOrgValidator struct {
	duplicateValidator webhooks.NameValidator
	placementValidator webhooks.NamespaceValidator
}

var _ webhook.CustomValidator = &CFOrgValidator{}

func NewCFOrgValidator(duplicateValidator webhooks.NameValidator, placementValidator webhooks.NamespaceValidator) *CFOrgValidator {
	return &CFOrgValidator{
		duplicateValidator: duplicateValidator,
		placementValidator: placementValidator,
	}
}

func (v *CFOrgValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFOrg{}).
		WithValidator(v).
		Complete()
}

func (v *CFOrgValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	org, ok := obj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	if len(org.Name) > maxLabelLength {
		return nil, errors.New("org name cannot be longer than 63 chars")
	}

	err := v.placementValidator.ValidateOrgCreate(*org)
	if err != nil {
		cfOrgLog.Info(err.Error())
		return nil, err.ExportJSONError()
	}

	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfOrgLog, org.Namespace, org)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFOrgValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	org, ok := obj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	if !org.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	oldOrg, ok := oldObj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateUpdate(ctx, cfOrgLog, org.Namespace, oldOrg, org)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}

func (v *CFOrgValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	org, ok := obj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfOrgLog, org.Namespace, org)
	if validationErr != nil {
		return nil, validationErr.ExportJSONError()
	}

	return nil, nil
}
