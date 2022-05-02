package workloads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-workloads-cloudfoundry-org-v1alpha1-cfspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=workloads.cloudfoundry.org,resources=cfspaces,verbs=create;update;delete,versions=v1alpha1,name=vcfspace.workloads.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

const CFSpaceEntityType = "cfspace"

var spaceLogger = logf.Log.WithName("cfspace-validate")

type CFSpaceValidation struct {
	duplicateSpaceValidator NameValidator
	decoder                 *admission.Decoder
}

func NewCFSpaceValidation(duplicateSpaceValidator NameValidator) *CFSpaceValidation {
	return &CFSpaceValidation{
		duplicateSpaceValidator: duplicateSpaceValidator,
	}
}

func (v *CFSpaceValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-workloads-cloudfoundry-org-v1alpha1-cfspace", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFSpaceValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	var handler cfSpaceHandler

	cfSpace := &workloadsv1alpha1.CFSpace{}
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := v.decoder.Decode(req, cfSpace); err != nil {
			spaceLogger.Error(err, "failed to decode CFSpace", "request", req)
			return admission.Denied(webhooks.AdmissionUnknownErrorReason())
		}

		var err error
		handler, err = v.newHandler()
		if err != nil {
			return admission.Denied(webhooks.AdmissionUnknownErrorReason())
		}

	}

	oldCFSpace := &workloadsv1alpha1.CFSpace{}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		if err := v.decoder.DecodeRaw(req.OldObject, oldCFSpace); err != nil {
			spaceLogger.Error(err, "failed to decode old CFSpace", "request", req)
			return admission.Denied(webhooks.AdmissionUnknownErrorReason())
		}

		var err error
		handler, err = v.newHandler()
		if err != nil {
			return admission.Allowed("")
		}

	}

	switch req.Operation {
	case admissionv1.Create:
		return handler.handleCreate(ctx, cfSpace)

	case admissionv1.Update:
		return handler.handleUpdate(ctx, oldCFSpace, cfSpace)

	case admissionv1.Delete:
		return handler.handleDelete(ctx, oldCFSpace)
	}

	spaceLogger.Info("unexpected operation", "operation", req.Operation)
	return admission.Denied(webhooks.AdmissionUnknownErrorReason())
}

func (v *CFSpaceValidation) newHandler() (cfSpaceHandler, error) {
	return NewCFSpaceHandler(
		spaceLogger.WithValues("entityType", CFSpaceEntityType),
		v.duplicateSpaceValidator,
		SpaceNameLabel,
		webhooks.ValidationError{Type: DuplicateSpaceNameErrorType, Message: duplicateSpaceNameErrorMessage},
	), nil
}

func (h *cfSpaceHandler) RenderDuplicateError(duplicateName string) string {
	formattedDuplicateError := webhooks.ValidationError{
		Type:    h.duplicateError.Type,
		Message: fmt.Sprintf(h.duplicateError.Message, duplicateName),
	}
	return formattedDuplicateError.Marshal()
}

type cfSpaceHandler struct {
	duplicateValidator NameValidator
	nameLabel          string
	duplicateError     webhooks.ValidationError
	logger             logr.Logger
}

func NewCFSpaceHandler(
	logger logr.Logger,
	duplicateValidator NameValidator,
	nameLabel string,
	duplicateError webhooks.ValidationError,
) cfSpaceHandler {
	return cfSpaceHandler{
		duplicateValidator: duplicateValidator,
		nameLabel:          nameLabel,
		duplicateError:     duplicateError,
		logger:             logger,
	}
}

func (h cfSpaceHandler) handleCreate(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace) admission.Response {
	spaceName := strings.ToLower(cfSpace.Spec.DisplayName)
	if err := h.duplicateValidator.ValidateCreate(ctx, h.logger, cfSpace.Namespace, spaceName); err != nil {
		if errors.Is(err, webhooks.ErrorDuplicateName) {
			return admission.Denied(h.RenderDuplicateError(spaceName))
		}

		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (h cfSpaceHandler) handleUpdate(ctx context.Context, oldCFSpace, newCFSpace *workloadsv1alpha1.CFSpace) admission.Response {
	newSpaceName := strings.ToLower(newCFSpace.Spec.DisplayName)
	oldSpaceName := strings.ToLower(oldCFSpace.Spec.DisplayName)
	if err := h.duplicateValidator.ValidateUpdate(ctx, h.logger, oldCFSpace.Namespace, oldSpaceName, newSpaceName); err != nil {
		if errors.Is(err, webhooks.ErrorDuplicateName) {
			return admission.Denied(h.RenderDuplicateError(newSpaceName))
		}

		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (h cfSpaceHandler) handleDelete(ctx context.Context, oldCFSpace *workloadsv1alpha1.CFSpace) admission.Response {
	if err := h.duplicateValidator.ValidateDelete(ctx, h.logger, oldCFSpace.Namespace, oldCFSpace.Spec.DisplayName); err != nil {
		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (v *CFSpaceValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
