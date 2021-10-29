package workloads

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/registry"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=create;update;delete,versions=v1alpha2,name=vsubns.kb.io,admissionReviewVersions={v1,v1beta1}
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;delete;patch

const (
	OrgNameLabel   = "cloudfoundry.org/org-name"
	SpaceNameLabel = "cloudfoundry.org/space-name"
)

type SubnamespaceAnchorValidation struct {
	decoder   *admission.Decoder
	registrar *registry.Registrar
	logger    logr.Logger
}

type mapToName func(*v1alpha2.SubnamespaceAnchor) string

var getOrgName mapToName = func(anchor *v1alpha2.SubnamespaceAnchor) string {
	return anchor.Labels[OrgNameLabel]
}

var getSpaceName mapToName = func(anchor *v1alpha2.SubnamespaceAnchor) string {
	return anchor.Labels[SpaceNameLabel]
}

func NewSubnamespaceAnchorValidation() *SubnamespaceAnchorValidation {
	return &SubnamespaceAnchorValidation{
		logger: logf.Log.WithName("subns-validate"),
	}
}

func (v *SubnamespaceAnchorValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor", &webhook.Admission{Handler: v})
	v.registrar = registry.NewRegistrar(mgr.GetClient())

	return nil
}

func (v *SubnamespaceAnchorValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return v.handleDelete(ctx, req)
	}

	anchor := &v1alpha2.SubnamespaceAnchor{}
	err := v.decoder.Decode(req, anchor)
	if err != nil {
		v.logger.Info("failed to decode subnamespace anchor", "error", err.Error())
		return admission.Denied("failed to decode subnamespace anchor")
	}

	if anchor.Labels[OrgNameLabel] == "" && anchor.Labels[SpaceNameLabel] == "" {
		return admission.Allowed("")
	}

	if anchor.Labels[OrgNameLabel] != "" && anchor.Labels[SpaceNameLabel] != "" {
		return admission.Denied("cannot have both org and space labels set")
	}

	if req.DryRun != nil && *req.DryRun {
		return admission.Allowed("not checking name uniqueness to avoid dry-run side effects")
	}

	anchorType := registry.OrgType
	dupError := DuplicateOrgNameError
	name := anchor.Labels[OrgNameLabel]
	getName := getOrgName
	if anchor.Labels[SpaceNameLabel] != "" {
		anchorType = registry.SpaceType
		dupError = DuplicateSpaceNameError
		name = anchor.Labels[SpaceNameLabel]
		getName = getSpaceName
	}

	switch req.Operation {
	case admissionv1.Create:
		return v.handleCreate(ctx, anchor, anchorType, dupError, name)
	case admissionv1.Update:
		var oldAnchor v1alpha2.SubnamespaceAnchor
		if err := v.decoder.DecodeRaw(req.OldObject, &oldAnchor); err != nil {
			v.logger.Info("failed to decode old object", "error", err)
			return admission.Denied("failed to decode subnamespace anchor")
		}
		return v.handleUpdate(ctx, anchor, &oldAnchor, anchorType, dupError, getName)
	}

	return admission.Allowed("")
}

func (v *SubnamespaceAnchorValidation) handleCreate(ctx context.Context, anchor *v1alpha2.SubnamespaceAnchor, anchorType registry.RegistryType, dupError ValidationErrorCode, name string) admission.Response {
	logger := v.logger.WithValues("anchorType", anchorType, "anchorNamespace", anchor.Namespace, "anchorName", name)
	err := v.registrar.TryRegister(ctx, anchorType, anchor, name)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info(dupError.GetMessage())
			return admission.Denied(dupError.Marshal())
		}
		logger.Info("creating lease record failed", "error", err)
		return admission.Denied("failed trying to register name")
	}

	return admission.Allowed("")
}

func (v *SubnamespaceAnchorValidation) handleUpdate(
	ctx context.Context,
	anchor, oldAnchor *v1alpha2.SubnamespaceAnchor,
	anchorType registry.RegistryType,
	dupError ValidationErrorCode,
	getName mapToName,
) admission.Response {

	anchorName := getName(anchor)
	oldAnchorName := getName(oldAnchor)
	if anchorName == oldAnchorName {
		return admission.Allowed("")
	}

	logger := v.logger.WithValues("anchorType", anchorType, "anchorNamespace", oldAnchor.Namespace, "oldAnchorName", oldAnchorName, "newAnchorName", anchorName)
	logger.Info("updating name")

	if err := v.registrar.TryClaimLease(ctx, anchorType, oldAnchor, oldAnchorName); err != nil {
		logger.Info("failed to obtain lease on old record", "err", err)
		return admission.Denied("cannot lock old record")
	}

	if err := v.registrar.TryRegister(ctx, anchorType, anchor, anchorName); err != nil {
		if err := v.registrar.ReleaseLease(ctx, anchorType, oldAnchor, oldAnchorName); err != nil {
			logger.Info("failed to release lease", "error", err)
		}
		if errors.IsAlreadyExists(err) {
			logger.Info(dupError.GetMessage())
			return admission.Denied(dupError.Marshal())
		}
		return admission.Denied("failed trying to register name")
	}

	if err := v.registrar.DeleteRecordFor(ctx, anchorType, oldAnchor.Namespace, oldAnchorName); err != nil {
		logger.Error(err, "failed to delete registration record", "anchorType", anchorType, "anchorNamespace", oldAnchor.Namespace, "anchorName", oldAnchorName)
	}

	return admission.Allowed("")
}

func (v *SubnamespaceAnchorValidation) handleDelete(ctx context.Context, req admission.Request) admission.Response {
	var oldAnchor v1alpha2.SubnamespaceAnchor
	if err := v.decoder.DecodeRaw(req.OldObject, &oldAnchor); err != nil {
		return admission.Denied("failed to decode subnamespace anchor")
	}

	anchorType := registry.OrgType
	name := oldAnchor.Labels[OrgNameLabel]
	if oldAnchor.Labels[SpaceNameLabel] != "" {
		anchorType = registry.SpaceType
		name = oldAnchor.Labels[SpaceNameLabel]
	}

	err := v.registrar.DeleteRecordFor(ctx, anchorType, oldAnchor.Namespace, name)
	if err != nil {
		v.logger.Error(err, "failed to delete register record", "namespace", oldAnchor.Namespace, "name", name)
	}
	return admission.Allowed("")
}

// Allow mgr to inject decoder
func (v *SubnamespaceAnchorValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
