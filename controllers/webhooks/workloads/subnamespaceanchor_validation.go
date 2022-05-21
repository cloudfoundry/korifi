package workloads

import (
	"context"
	"errors"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor,mutating=false,failurePolicy=fail,sideEffects=None,groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=create;update;delete,versions=v1alpha2,name=vsubns.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

const (
	OrgNameLabel    = "cloudfoundry.org/org-name"
	SpaceNameLabel  = "cloudfoundry.org/space-name"
	OrgEntityType   = "org"
	SpaceEntityType = "space"

	DuplicateOrgNameErrorType = "DuplicateOrgNameError"
	// Note: the cf cli expects the specfic text `Organization '.*' already exists.` in the error and ignores the error if it matches it.
	duplicateOrgNameErrorMessage = "Organization '%s' already exists."
	DuplicateSpaceNameErrorType  = "DuplicateSpaceNameError"
	// Note: the cf cli expects the specific text `Name must be unique per organization` in the error and ignores the error if it matches it.
	duplicateSpaceNameErrorMessage = "Space '%s' already exists. Name must be unique per organization."
)

var subnsLogger = logf.Log.WithName("subns-validate")

type SubnamespaceAnchorValidation struct {
	duplicateOrgValidator   NameValidator
	duplicateSpaceValidator NameValidator
	decoder                 *admission.Decoder
}

func NewSubnamespaceAnchorValidation(duplicateOrgValidator, duplicateSpaceValidator NameValidator) *SubnamespaceAnchorValidation {
	return &SubnamespaceAnchorValidation{
		duplicateOrgValidator:   duplicateOrgValidator,
		duplicateSpaceValidator: duplicateSpaceValidator,
	}
}

func (v *SubnamespaceAnchorValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor", &webhook.Admission{Handler: v})

	return nil
}

func (v *SubnamespaceAnchorValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	var handler subnamespaceAnchorHandler

	anchor := &v1alpha2.SubnamespaceAnchor{}
	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := v.decoder.Decode(req, anchor); err != nil {
			subnsLogger.Error(err, "failed to decode subnamespace anchor", "request", req)
			return admission.Denied(webhooks.AdmissionUnknownErrorReason())
		}

		if valid, response := v.validateLabels(anchor); !valid {
			return response
		}

		var err error
		handler, err = v.newHandler(anchor)
		if err != nil {
			return admission.Denied(webhooks.AdmissionUnknownErrorReason())
		}

	}

	oldAnchor := &v1alpha2.SubnamespaceAnchor{}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		if err := v.decoder.DecodeRaw(req.OldObject, oldAnchor); err != nil {
			subnsLogger.Error(err, "failed to decode old subnamespace anchor", "request", req)
			return admission.Denied(webhooks.AdmissionUnknownErrorReason())
		}

		if valid, _ := v.validateLabels(oldAnchor); !valid {
			return admission.Allowed("")
		}

		var err error
		handler, err = v.newHandler(oldAnchor)
		if err != nil {
			return admission.Allowed("")
		}

	}

	switch req.Operation {
	case admissionv1.Create:
		return handler.handleCreate(ctx, anchor)

	case admissionv1.Update:
		return handler.handleUpdate(ctx, oldAnchor, anchor)

	case admissionv1.Delete:
		return handler.handleDelete(ctx, oldAnchor)
	}

	subnsLogger.Info("unexpected operation", "operation", req.Operation)
	return admission.Denied(webhooks.AdmissionUnknownErrorReason())
}

func (v *SubnamespaceAnchorValidation) validateLabels(anchor *v1alpha2.SubnamespaceAnchor) (bool, admission.Response) {
	if anchor.Labels[OrgNameLabel] == "" && anchor.Labels[SpaceNameLabel] == "" {
		return false, admission.Allowed("")
	}

	if anchor.Labels[OrgNameLabel] != "" && anchor.Labels[SpaceNameLabel] != "" {
		subnsLogger.Info("cannot have both org and space labels set", "anchor", anchor)
		return false, admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return true, admission.Response{}
}

// newHandler must be called after v.validateLabels() has ensured only org or space label is non-empty
func (v *SubnamespaceAnchorValidation) newHandler(anchor *v1alpha2.SubnamespaceAnchor) (subnamespaceAnchorHandler, error) {
	switch {
	case anchor.Labels[OrgNameLabel] != "" && anchor.Labels[SpaceNameLabel] == "":
		return NewSubnamespaceAnchorHandler(
			subnsLogger.WithValues("entityType", OrgEntityType),
			v.duplicateOrgValidator,
			OrgNameLabel,
			webhooks.ValidationError{Type: DuplicateOrgNameErrorType, Message: duplicateOrgNameErrorMessage},
		), nil

	case anchor.Labels[SpaceNameLabel] != "" && anchor.Labels[OrgNameLabel] == "":
		return NewSubnamespaceAnchorHandler(
			subnsLogger.WithValues("entityType", SpaceEntityType),
			v.duplicateSpaceValidator,
			SpaceNameLabel,
			webhooks.ValidationError{Type: DuplicateSpaceNameErrorType, Message: duplicateSpaceNameErrorMessage},
		), nil

	default:
		err := errors.New("expected exactly 1 of org label and space label to be set")
		subnsLogger.Error(err, "could not decide whether to create org or space handler", "anchor", anchor)
		return subnamespaceAnchorHandler{}, err
	}
}

func (h *subnamespaceAnchorHandler) RenderDuplicateError(duplicateName string) string {
	formattedDuplicateError := webhooks.ValidationError{
		Type:    h.duplicateError.Type,
		Message: fmt.Sprintf(h.duplicateError.Message, duplicateName),
	}
	return formattedDuplicateError.Marshal()
}

type subnamespaceAnchorHandler struct {
	duplicateValidator NameValidator
	nameLabel          string
	duplicateError     webhooks.ValidationError
	logger             logr.Logger
}

func NewSubnamespaceAnchorHandler(
	logger logr.Logger,
	duplicateValidator NameValidator,
	nameLabel string,
	duplicateError webhooks.ValidationError,
) subnamespaceAnchorHandler {
	return subnamespaceAnchorHandler{
		duplicateValidator: duplicateValidator,
		nameLabel:          nameLabel,
		duplicateError:     duplicateError,
		logger:             logger,
	}
}

func (h subnamespaceAnchorHandler) handleCreate(ctx context.Context, anchor *v1alpha2.SubnamespaceAnchor) admission.Response {
	anchorName := h.getName(anchor)
	if err := h.duplicateValidator.ValidateCreate(ctx, h.logger, anchor.Namespace, anchorName); err != nil {
		if errors.Is(err, webhooks.ErrorDuplicateName) {
			return admission.Denied(h.RenderDuplicateError(anchorName))
		}

		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (h subnamespaceAnchorHandler) handleUpdate(ctx context.Context, oldAnchor, newAnchor *v1alpha2.SubnamespaceAnchor) admission.Response {
	newAnchorName := h.getName(newAnchor)
	if err := h.duplicateValidator.ValidateUpdate(ctx, h.logger, oldAnchor.Namespace, h.getName(oldAnchor), newAnchorName); err != nil {
		if errors.Is(err, webhooks.ErrorDuplicateName) {
			return admission.Denied(h.RenderDuplicateError(newAnchorName))
		}

		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (h subnamespaceAnchorHandler) handleDelete(ctx context.Context, oldAnchor *v1alpha2.SubnamespaceAnchor) admission.Response {
	if err := h.duplicateValidator.ValidateDelete(ctx, h.logger, oldAnchor.Namespace, h.getName(oldAnchor)); err != nil {
		return admission.Denied(webhooks.AdmissionUnknownErrorReason())
	}

	return admission.Allowed("")
}

func (h subnamespaceAnchorHandler) getName(anchor *v1alpha2.SubnamespaceAnchor) string {
	return anchor.Labels[h.nameLabel]
}

// Allow mgr to inject decoder
func (v *SubnamespaceAnchorValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
