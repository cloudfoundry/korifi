package security_groups

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

const SecurityGroupEnyityType = "securityGroup"

var cfsecuritygrouplog = logf.Log.WithName("cfsecuritygroup-validation")

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfsecuritygroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfsecuritygroups,verbs=create;update;delete,versions=v1alpha1,name=vcfsecuritygroup.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type Validator struct {
	duplicateValidator webhooks.NameValidator
	rootNamespace      string
}

var _ webhook.CustomValidator = &Validator{}

func NewValidator(duplicateValidator webhooks.NameValidator, rootNamespace string) *Validator {
	return &Validator{
		duplicateValidator: duplicateValidator,
		rootNamespace:      rootNamespace,
	}
}

func (v *Validator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFSecurityGroup{}).
		WithValidator(v).
		Complete()
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	securityGroup, ok := obj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateCreate(ctx, cfsecuritygrouplog, v.rootNamespace, securityGroup)
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	securityGroup, ok := obj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", obj))
	}

	if !securityGroup.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	oldSecurityGroup, ok := oldObj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", oldObj))
	}

	return nil, v.duplicateValidator.ValidateUpdate(ctx, cfsecuritygrouplog, v.rootNamespace, oldSecurityGroup, securityGroup)
}

func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	securityGroup, ok := obj.(*korifiv1alpha1.CFSecurityGroup)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a CFSecurityGroup but got a %T", obj))
	}

	return nil, v.duplicateValidator.ValidateDelete(ctx, cfsecuritygrouplog, v.rootNamespace, securityGroup)
}
