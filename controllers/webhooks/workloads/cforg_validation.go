package workloads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	CFOrgEntityType          = "cforg"
	OrgDecodingErrorType     = "OrgDecodingError"
	DuplicateOrgErrorType    = "DuplicateOrgNameError"
	OrgPlacementErrorType    = "OrgPlacementError"
	OrgPlacementErrorMessage = "Organization '%s' must be placed in the root 'cf' namespace"
)

var cfOrgLog = logf.Log.WithName("cforg-validate")

//+kubebuilder:webhook:path=/validate-workloads-cloudfoundry-org-v1alpha1-cforg,mutating=false,failurePolicy=fail,sideEffects=None,groups=workloads.cloudfoundry.org,resources=cforgs,verbs=create;update;delete,versions=v1alpha1,name=vcforg.workloads.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFOrgValidation struct {
	decoder            *admission.Decoder
	duplicateValidator NameValidator
	placementValidator PlacementValidator
}

func NewCFOrgValidation(duplicateValidator NameValidator, placementValidator PlacementValidator) *CFOrgValidation {
	return &CFOrgValidation{
		duplicateValidator: duplicateValidator,
		placementValidator: placementValidator,
	}
}

func (v *CFOrgValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-workloads-cloudfoundry-org-v1alpha1-cforg", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFOrgValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	cfOrgLog.Info("Validate", "name", req.Name)

	var cfOrg, oldCFOrg v1alpha1.CFOrg
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := v.decoder.Decode(req, &cfOrg)
		if err != nil { // untested
			errMessage := "Error while decoding CFOrg object"
			cfOrgLog.Error(err, errMessage)
			return admission.Denied(webhooks.ValidationError{Type: OrgDecodingErrorType, Message: errMessage}.Marshal())
		}
	}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		err := v.decoder.DecodeRaw(req.OldObject, &oldCFOrg)
		if err != nil { // untested
			errMessage := "Error while decoding old CFOrg object"
			cfOrgLog.Error(err, errMessage)
			return admission.Denied(webhooks.ValidationError{Type: OrgDecodingErrorType, Message: errMessage}.Marshal())
		}
	}

	if req.Operation == admissionv1.Create {
		err := v.placementValidator.ValidateOrgCreate(cfOrg)
		if err != nil {
			cfOrgLog.Error(err, err.Error())
			return admission.Denied(webhooks.ValidationError{Type: OrgPlacementErrorType, Message: err.Error()}.Marshal())
		}
	}

	var validatorErr error
	cfOrgNameLeaseValue := strings.ToLower(cfOrg.Spec.DisplayName)
	switch req.Operation {
	case admissionv1.Create:
		validatorErr = v.duplicateValidator.ValidateCreate(ctx, cfOrgLog, cfOrg.Namespace, cfOrgNameLeaseValue)

	case admissionv1.Update:
		oldCFOrgNameLeaseValue := strings.ToLower(oldCFOrg.Spec.DisplayName)
		validatorErr = v.duplicateValidator.ValidateUpdate(ctx, cfOrgLog, cfOrg.Namespace, oldCFOrgNameLeaseValue, cfOrgNameLeaseValue)

	case admissionv1.Delete:
		oldCFOrgNameLeaseValue := strings.ToLower(oldCFOrg.Spec.DisplayName)
		validatorErr = v.duplicateValidator.ValidateDelete(ctx, cfOrgLog, oldCFOrg.Namespace, oldCFOrgNameLeaseValue)
	}

	if validatorErr != nil {
		if errors.Is(validatorErr, webhooks.ErrorDuplicateName) {
			errorMessage := fmt.Sprintf("Organization '%s' already exists.", cfOrg.Spec.DisplayName)
			return admission.Denied(webhooks.ValidationError{Type: DuplicateOrgErrorType, Message: errorMessage}.Marshal())
		}

		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (v *CFOrgValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
