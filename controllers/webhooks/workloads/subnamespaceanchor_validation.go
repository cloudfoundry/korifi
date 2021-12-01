package workloads

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor,mutating=false,failurePolicy=fail,sideEffects=None,groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=create;update;delete,versions=v1alpha2,name=vsubns.workloads.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;patch;delete

const (
	OrgNameLabel    = "cloudfoundry.org/org-name"
	SpaceNameLabel  = "cloudfoundry.org/space-name"
	OrgEntityType   = "org"
	SpaceEntityType = "space"
)

var subnsLogger = logf.Log.WithName("subns-validate")

//counterfeiter:generate -o fake -fake-name NameRegistry . NameRegistry

type NameRegistry interface {
	RegisterName(ctx context.Context, namespace, name string) error
	DeregisterName(ctx context.Context, namespace, name string) error
	TryLockName(ctx context.Context, namespace, name string) error
	UnlockName(ctx context.Context, namespace, name string) error
}

type SubnamespaceAnchorValidation struct {
	orgNameRegistry   NameRegistry
	spaceNameRegistry NameRegistry
	decoder           *admission.Decoder
}

func NewSubnamespaceAnchorValidation(orgNameRegistry, spaceNameRegistry NameRegistry) *SubnamespaceAnchorValidation {
	return &SubnamespaceAnchorValidation{
		orgNameRegistry:   orgNameRegistry,
		spaceNameRegistry: spaceNameRegistry,
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
			return admission.Denied(UnknownError.Marshal())
		}

		if valid, response := v.validateLabels(anchor); !valid {
			return response
		}

		var err error
		handler, err = v.newHandler(anchor)
		if err != nil {
			return admission.Denied(UnknownError.Marshal())
		}

	}

	oldAnchor := &v1alpha2.SubnamespaceAnchor{}
	if req.Operation == admissionv1.Update || req.Operation == admissionv1.Delete {
		if err := v.decoder.DecodeRaw(req.OldObject, oldAnchor); err != nil {
			subnsLogger.Error(err, "failed to decode old subnamespace anchor", "request", req)
			return admission.Denied(UnknownError.Marshal())
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
		if handler.nameHasNotChanged(oldAnchor, anchor) {
			return admission.Allowed("")
		}
		return handler.handleUpdate(ctx, oldAnchor, anchor)

	case admissionv1.Delete:
		return handler.handleDelete(ctx, oldAnchor)
	}

	subnsLogger.Info("unexpected operation", "operation", req.Operation)
	return admission.Denied(UnknownError.Marshal())
}

func (v *SubnamespaceAnchorValidation) validateLabels(anchor *v1alpha2.SubnamespaceAnchor) (bool, admission.Response) {
	if anchor.Labels[OrgNameLabel] == "" && anchor.Labels[SpaceNameLabel] == "" {
		return false, admission.Allowed("")
	}

	if anchor.Labels[OrgNameLabel] != "" && anchor.Labels[SpaceNameLabel] != "" {
		subnsLogger.Info("cannot have both org and space labels set", "anchor", anchor)
		return false, admission.Denied(UnknownError.Marshal())
	}

	return true, admission.Response{}
}

// newHandler must be called after v.validateLabels() has ensured only org or space label is non-empty
func (v *SubnamespaceAnchorValidation) newHandler(anchor *v1alpha2.SubnamespaceAnchor) (subnamespaceAnchorHandler, error) {
	switch {
	case anchor.Labels[OrgNameLabel] != "" && anchor.Labels[SpaceNameLabel] == "":
		return subnamespaceAnchorHandler{
			nameRegistry:   v.orgNameRegistry,
			nameLabel:      OrgNameLabel,
			duplicateError: DuplicateOrgNameError,
			logger:         subnsLogger.WithValues("entityType", OrgEntityType),
		}, nil

	case anchor.Labels[SpaceNameLabel] != "" && anchor.Labels[OrgNameLabel] == "":
		return subnamespaceAnchorHandler{
			nameRegistry:   v.spaceNameRegistry,
			nameLabel:      SpaceNameLabel,
			duplicateError: DuplicateSpaceNameError,
			logger:         subnsLogger.WithValues("entityType", SpaceEntityType),
		}, nil

	default:
		err := errors.New("expected exactly 1 of org label and space label to be set")
		subnsLogger.Error(err, "could not decide whether to create org or space handler", "anchor", anchor)
		return subnamespaceAnchorHandler{}, err
	}
}

type subnamespaceAnchorHandler struct {
	nameRegistry   NameRegistry
	nameLabel      string
	duplicateError ValidationErrorCode
	logger         logr.Logger
}

func (h subnamespaceAnchorHandler) handleCreate(ctx context.Context, anchor *v1alpha2.SubnamespaceAnchor) admission.Response {
	err := h.nameRegistry.RegisterName(ctx, anchor.Namespace, h.getName(anchor))
	if k8serrors.IsAlreadyExists(err) {
		h.logger.Info(h.duplicateError.GetMessage(),
			"name", h.getName(anchor),
			"namespace", anchor.Namespace,
		)
		return admission.Denied(h.duplicateError.Marshal())
	}
	if err != nil {
		h.logger.Info("failed to register name during create",
			"error", err,
			"name", h.getName(anchor),
			"namespace", anchor.Namespace,
		)
		return admission.Denied(UnknownError.Marshal())
	}

	return admission.Allowed("")
}

func (h subnamespaceAnchorHandler) handleUpdate(ctx context.Context, oldAnchor, newAnchor *v1alpha2.SubnamespaceAnchor) admission.Response {
	err := h.nameRegistry.TryLockName(ctx, oldAnchor.Namespace, h.getName(oldAnchor))
	if err != nil {
		h.logger.Info("failed to acquire lock on old name during update",
			"error", err,
			"name", h.getName(oldAnchor),
			"namespace", oldAnchor.Namespace,
		)
		return admission.Denied(UnknownError.Marshal())
	}

	err = h.nameRegistry.RegisterName(ctx, newAnchor.Namespace, h.getName(newAnchor))
	if err != nil {
		// cannot register new name, so unlock old registry entry allowing future renames
		unlockErr := h.nameRegistry.UnlockName(ctx, oldAnchor.Namespace, h.getName(oldAnchor))
		if unlockErr != nil {
			// A locked registry entry will remain, so future name updates will fail until operator intervenes
			h.logger.Error(unlockErr, "failed to release registry lock on old name",
				"name", h.getName(oldAnchor),
				"namespace", oldAnchor.Namespace,
			)
		}

		if k8serrors.IsAlreadyExists(err) {
			h.logger.Info(h.duplicateError.GetMessage(),
				"name", h.getName(newAnchor),
				"namespace", newAnchor.Namespace,
			)
			return admission.Denied(h.duplicateError.Marshal())
		}

		h.logger.Error(err, "failed to register name",
			"name", h.getName(newAnchor),
			"namespace", newAnchor.Namespace,
		)

		return admission.Denied(UnknownError.Marshal())
	}

	err = h.nameRegistry.DeregisterName(ctx, oldAnchor.Namespace, h.getName(oldAnchor))
	if err != nil {
		// We cannot unclaim the old name. It will remain claimed until an operator intervenes.
		h.logger.Error(err, "failed to deregister old name during update",
			"name", h.getName(newAnchor),
			"namespace", newAnchor.Namespace,
		)
	}

	return admission.Allowed("")
}

func (h subnamespaceAnchorHandler) handleDelete(ctx context.Context, oldAnchor *v1alpha2.SubnamespaceAnchor) admission.Response {
	err := h.nameRegistry.DeregisterName(ctx, oldAnchor.Namespace, h.getName(oldAnchor))
	if k8serrors.IsNotFound(err) {
		h.logger.Info("cannot deregister name: registry entry for name not found",
			"namespace", oldAnchor.Namespace,
			"name", h.getName(oldAnchor),
		)
		return admission.Allowed("")
	}

	if err != nil {
		h.logger.Error(err, "failed to deregister name during delete",
			"namespace", oldAnchor.Namespace,
			"name", h.getName(oldAnchor),
		)
		return admission.Denied(UnknownError.Marshal())
	}

	return admission.Allowed("")
}

func (h subnamespaceAnchorHandler) nameHasNotChanged(oldAnchor, anchor *v1alpha2.SubnamespaceAnchor) bool {
	return h.getName(oldAnchor) == h.getName(anchor)
}

func (h subnamespaceAnchorHandler) getName(anchor *v1alpha2.SubnamespaceAnchor) string {
	return anchor.Labels[h.nameLabel]
}

// Allow mgr to inject decoder
func (v *SubnamespaceAnchorValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
