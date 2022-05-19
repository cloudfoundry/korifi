package workloads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-korifi-cloudfoundry-org-v1alpha1-cfspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfspaces,verbs=create;update;delete,versions=v1alpha1,name=vcfspace.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

const CFSpaceEntityType = "cfspace"

var spaceLogger = logf.Log.WithName("cfspace-validate")

type CFSpaceValidation struct {
	duplicateSpaceValidator NameValidator
	decoder                 *admission.Decoder
	placementValidator      PlacementValidator
}

func NewCFSpaceValidation(duplicateSpaceValidator NameValidator, placementValidator PlacementValidator) *CFSpaceValidation {
	return &CFSpaceValidation{
		duplicateSpaceValidator: duplicateSpaceValidator,
		placementValidator:      placementValidator,
	}
}

func (v *CFSpaceValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-korifi-cloudfoundry-org-v1alpha1-cfspace", &webhook.Admission{Handler: v})

	return nil
}

func (v *CFSpaceValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	var handler cfSpaceHandler

	cfSpace := &v1alpha1.CFSpace{}
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

	oldCFSpace := &v1alpha1.CFSpace{}
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
		webhooks.ValidationError{Type: DuplicateSpaceNameErrorType, Message: duplicateSpaceNameErrorMessage},
		v.duplicateSpaceValidator,
		spaceLogger.WithValues("entityType", CFSpaceEntityType),
		SpaceNameLabel,
		v.placementValidator,
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
	duplicateError     webhooks.ValidationError
	duplicateValidator NameValidator
	logger             logr.Logger
	nameLabel          string
	placementValidator PlacementValidator
}

func NewCFSpaceHandler(
	duplicateError webhooks.ValidationError,
	duplicateValidator NameValidator,
	logger logr.Logger,
	nameLabel string,
	placementValidator PlacementValidator,
) cfSpaceHandler {
	return cfSpaceHandler{
		duplicateError:     duplicateError,
		duplicateValidator: duplicateValidator,
		logger:             logger,
		nameLabel:          nameLabel,
		placementValidator: placementValidator,
	}
}

func (h cfSpaceHandler) handleCreate(ctx context.Context, cfSpace *v1alpha1.CFSpace) admission.Response {
	spaceName := strings.ToLower(cfSpace.Spec.DisplayName)
	if err := h.duplicateValidator.ValidateCreate(ctx, h.logger, cfSpace.Namespace, spaceName); err != nil {
		if errors.Is(err, webhooks.ErrorDuplicateName) {
			return admission.Denied(h.RenderDuplicateError(spaceName))
		}

		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	if err := h.placementValidator.ValidateSpaceCreate(*cfSpace); err != nil {
		return admission.Denied(err.Error())
	}

	return admission.Allowed("")
}

func (h cfSpaceHandler) handleUpdate(ctx context.Context, oldCFSpace, newCFSpace *v1alpha1.CFSpace) admission.Response {
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

func (h cfSpaceHandler) handleDelete(ctx context.Context, oldCFSpace *v1alpha1.CFSpace) admission.Response {
	if err := h.duplicateValidator.ValidateDelete(ctx, h.logger, oldCFSpace.Namespace, oldCFSpace.Spec.DisplayName); err != nil {
		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (v *CFSpaceValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
