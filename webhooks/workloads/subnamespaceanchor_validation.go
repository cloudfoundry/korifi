package workloads

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/registry"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor,mutating=false,failurePolicy=fail,sideEffects=NoneOnDryRun,groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=create;update;delete;connect,versions=v1alpha2,name=vsubns.kb.io,admissionReviewVersions={v1,v1beta1}
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;delete;patch

const (
	OrgNameLabel   = "cloudfoundry.org/org-name"
	SpaceNameLabel = "cloudfoundry.org/space-name"
)

var subnsLogger = logf.Log.WithName("subns-validate")

//counterfeiter:generate -o fake -fake-name SubnamespaceAnchorLister . SubnamespaceAnchorLister

type SubnamespaceAnchorLister interface {
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

type SubnamespaceAnchorValidation struct {
	lister    SubnamespaceAnchorLister
	decoder   *admission.Decoder
	registrar *registry.Registrar
}

type mapToName func(*v1alpha2.SubnamespaceAnchor) string

var getOrgName mapToName = func(anchor *v1alpha2.SubnamespaceAnchor) string {
	return anchor.Labels[OrgNameLabel]
}

var getSpaceName mapToName = func(anchor *v1alpha2.SubnamespaceAnchor) string {
	return anchor.Labels[SpaceNameLabel]
}

func NewSubnamespaceAnchorValidation(lister SubnamespaceAnchorLister) *SubnamespaceAnchorValidation {
	return &SubnamespaceAnchorValidation{
		lister: lister,
	}
}

func (v *SubnamespaceAnchorValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor", &webhook.Admission{Handler: v})
	v.registrar = registry.NewRegistrar(mgr.GetClient())

	return nil
}

func (v *SubnamespaceAnchorValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	time.Sleep(2 * time.Second)
	anchor := &v1alpha2.SubnamespaceAnchor{}
	if req.Operation == admissionv1.Delete {
		return v.handleDelete(ctx, req)
	}

	err := v.decoder.Decode(req, anchor)
	if err != nil {
		subnsLogger.Info("failed to decode subnamespace anchor", "error", err.Error())
		return admission.Errored(1, fmt.Errorf("failed to decode subnamespace anchor: %w", err))
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
			return admission.Errored(1, fmt.Errorf("failed to decode subnamespace anchor: %w", err))
		}
		return v.handleUpdate(ctx, anchor, &oldAnchor, anchorType, dupError, getName)
	}

	subnsLogger.Info("connect?", "req", req)
	return admission.Allowed("")
}

func (v *SubnamespaceAnchorValidation) handleCreate(ctx context.Context, anchor *v1alpha2.SubnamespaceAnchor, anchorType registry.RegistryType, dupError ValidationErrorCode, name string) admission.Response {
	err := v.registrar.TryRegister(ctx, anchorType, anchor, name)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			subnsLogger.Info(dupError.GetMessage(), "name", name)
			return admission.Denied(dupError.Marshal())
		}
		return admission.Errored(2, fmt.Errorf("failed trying to register name: %w", err))
	}

	return admission.Allowed("")
}

func (v *SubnamespaceAnchorValidation) handleUpdate(ctx context.Context, anchor, oldAnchor *v1alpha2.SubnamespaceAnchor, anchorType registry.RegistryType, dupError ValidationErrorCode, getName mapToName) admission.Response {
	anchorName := getName(anchor)
	oldAnchorName := getName(oldAnchor)
	if anchorName == oldAnchorName {
		return admission.Allowed("")
	}
	subnsLogger.Info("updating name", "oldName", oldAnchorName, "newName", anchorName)

	if err := v.registrar.TryClaimLease(ctx, anchorType, oldAnchor, oldAnchorName); err != nil {
		subnsLogger.Info("failed to obtain lease on old record", "err", err, "anchorType", anchorType, "anchorNamespace", oldAnchor.Namespace, "anchorName", oldAnchorName)
		return admission.Denied("cannot lock old record")
	}

	if err := v.registrar.TryRegister(ctx, anchorType, oldAnchor, oldAnchorName); err != nil {
		if errors.IsAlreadyExists(err) {
			subnsLogger.Info(dupError.GetMessage(), "name", anchorName)
			return admission.Denied(dupError.Marshal())
		}
		return admission.Errored(2, fmt.Errorf("failed trying to register name: %w", err))
	}

	if err := v.registrar.DeleteRecordFor(ctx, anchorType, oldAnchor.Namespace, oldAnchorName); err != nil {
		subnsLogger.Error(err, "failed to delete registration record", "anchorType", anchorType, "anchorNamespace", oldAnchor.Namespace, "anchorName", oldAnchorName)
	}

	return admission.Allowed("")
}

func (v *SubnamespaceAnchorValidation) handleDelete(ctx context.Context, req admission.Request) admission.Response {
	var oldAnchor v1alpha2.SubnamespaceAnchor
	if err := v.decoder.DecodeRaw(req.OldObject, &oldAnchor); err != nil {
		return admission.Errored(1, fmt.Errorf("failed to decode subnamespace anchor: %w", err))
	}

	anchorType := registry.OrgType
	name := oldAnchor.Labels[OrgNameLabel]
	if oldAnchor.Labels[SpaceNameLabel] != "" {
		anchorType = registry.SpaceType
		name = oldAnchor.Labels[SpaceNameLabel]
	}

	err := v.registrar.DeleteRecordFor(ctx, anchorType, oldAnchor.Namespace, name)
	if err != nil {
		subnsLogger.Error(err, "failed to delete register record", "namespace", oldAnchor.Namespace, "name", name)
	}
	return admission.Allowed("")
}

// Allow mgr to inject decoder
func (v *SubnamespaceAnchorValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
