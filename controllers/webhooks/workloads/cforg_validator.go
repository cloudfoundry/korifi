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
)

const (
	CFOrgEntityType      = "cforg"
	OrgDecodingErrorType = "OrgDecodingError"
	// Note: the cf cli expects the specfic text `Organization '.*' already exists.` in the error and ignores the error if it matches it.
	duplicateOrgNameErrorMessage = "Organization '%s' already exists."
)

var cfOrgLog = logf.Log.WithName("cforg-validate")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cforg,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cforgs,verbs=create;update;delete,versions=v1alpha1,name=vcforg.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

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

func (v *CFOrgValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	org, ok := obj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	err := v.placementValidator.ValidateOrgCreate(*org)
	if err != nil {
		cfOrgLog.Error(err, err.Error())
		return err.ExportJSONError()
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateOrgNameErrorMessage, org.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateCreate(ctx, cfOrgLog, org.Namespace, strings.ToLower(org.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func (v *CFOrgValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) error {
	oldOrg, ok := oldObj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	org, ok := obj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	duplicateErrorMessage := fmt.Sprintf(duplicateOrgNameErrorMessage, org.Spec.DisplayName)
	validationErr := v.duplicateValidator.ValidateUpdate(ctx, cfOrgLog, org.Namespace, strings.ToLower(oldOrg.Spec.DisplayName), strings.ToLower(org.Spec.DisplayName), duplicateErrorMessage)
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}

func (v *CFOrgValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	org, ok := obj.(*korifiv1alpha1.CFOrg)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a CFOrg but got a %T", obj))
	}

	validationErr := v.duplicateValidator.ValidateDelete(ctx, cfOrgLog, org.Namespace, strings.ToLower(org.Spec.DisplayName))
	if validationErr != nil {
		return validationErr.ExportJSONError()
	}

	return nil
}
