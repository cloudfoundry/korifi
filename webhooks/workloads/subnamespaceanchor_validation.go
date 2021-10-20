package workloads

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor,mutating=false,failurePolicy=fail,sideEffects=None,groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=create;update,versions=v1alpha2,name=vsubns.kb.io,admissionReviewVersions={v1,v1beta1}
//+kubebuilder:rbac:groups=hnc.x-k8s.io,resources=subnamespaceanchors,verbs=list;watch

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
	lister  SubnamespaceAnchorLister
	decoder *admission.Decoder
}

func NewSubnamespaceAnchorValidation(lister SubnamespaceAnchorLister) *SubnamespaceAnchorValidation {
	return &SubnamespaceAnchorValidation{
		lister: lister,
	}
}

func (v *SubnamespaceAnchorValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchor", &webhook.Admission{Handler: v})

	return nil
}

func (v *SubnamespaceAnchorValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	anchor := &v1alpha2.SubnamespaceAnchor{}
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

	existingItems := &v1alpha2.SubnamespaceAnchorList{}

	label := OrgNameLabel
	dupError := DuplicateOrgNameError
	if anchor.Labels[SpaceNameLabel] != "" {
		label = SpaceNameLabel
		dupError = DuplicateSpaceNameError
	}

	err = v.lister.List(ctx, existingItems, client.InNamespace(req.Namespace), client.MatchingLabels{label: anchor.Labels[label]})
	if err != nil {
		subnsLogger.Info("listing subnamespace anchors failed", "error", err.Error())
		return admission.Errored(2, fmt.Errorf("failed listing subnamespace anchors: %w", err))
	}

	var items []v1alpha2.SubnamespaceAnchor
	for _, item := range existingItems.Items {
		if item.Name != anchor.Name {
			items = append(items, item)
		}
	}

	if len(items) > 0 {
		subnsLogger.Info(dupError.GetMessage(), "name", anchor.Labels[label])
		return admission.Denied(dupError.Marshal())
	}

	return admission.Allowed("")
}

// Allow mgr to inject decoder
func (v *SubnamespaceAnchorValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
