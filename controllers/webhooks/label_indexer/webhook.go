package label_indexer

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-label-indexer,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes;cfapps;cfbuilds;cfdomains;cfpackages;cfprocesses;cfservicebindings;cfserviceinstances;cftasks,verbs=create;update,versions=v1alpha1,name=mcflabelindexer.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/rules"  //lint:ignore ST1001 for readability
	. "code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values" //lint:ignore ST1001 for readability
	"code.cloudfoundry.org/korifi/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type IndexingRule interface {
	Apply(obj map[string]any) (map[string]string, error)
}

type LabelIndexerWebhook struct {
	decoder       admission.Decoder
	indexingRules map[string][]IndexingRule
}

func NewWebhook() *LabelIndexerWebhook {
	return &LabelIndexerWebhook{
		indexingRules: map[string][]IndexingRule{
			"CFRoute": {
				LabelRule{Label: korifiv1alpha1.CFDomainGUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.spec.domainRef.name"))},
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
				LabelRule{Label: korifiv1alpha1.CFRouteHostLabelKey, IndexingFunc: Unquote(JSONValue("$.spec.host"))},
				LabelRule{Label: korifiv1alpha1.CFRoutePathLabelKey, IndexingFunc: SHA224(Unquote(JSONValue("$.spec.path")))},
				LabelRule{Label: korifiv1alpha1.CFRouteIsUnmappedLabelKey, IndexingFunc: IsEmptyValue(JSONValue("$.spec.destinations[*]"))},
				MultiLabelRule{LabelRules: DestinationAppGuidLabelRules},
			},
			"CFApp": {
				LabelRule{Label: korifiv1alpha1.GUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.metadata.name"))},
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
				LabelRule{Label: korifiv1alpha1.CFAppDisplayNameKey, IndexingFunc: SHA224(Unquote(JSONValue("$.spec.displayName")))},
			},
			"CFBuild": {
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
				LabelRule{Label: korifiv1alpha1.CFDropletGUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.metadata.name"))},
				LabelRule{Label: korifiv1alpha1.CFAppGUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.spec.appRef.name"))},
				LabelRule{Label: korifiv1alpha1.CFPackageGUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.spec.packageRef.name"))},
				LabelRule{Label: korifiv1alpha1.CFBuildStateLabelKey, IndexingFunc: Unquote(JSONValue("$.status.state"))},
			},
			"CFDomain": {
				LabelRule{Label: korifiv1alpha1.CFEncodedDomainNameLabelKey, IndexingFunc: SHA224(Unquote(JSONValue("$.spec.name")))},
			},
			"CFPackage": {
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
				LabelRule{Label: korifiv1alpha1.CFAppGUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.spec.appRef.name"))},
			},
			"CFProcess": {
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
			},
			"CFServiceInstance": {
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
			},
			"CFServiceBinding": {
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
			},
			"CFTask": {
				LabelRule{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
			},
		},
	}
}

func (r *LabelIndexerWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-label-indexer", &admission.Webhook{
		Handler: r,
	})
	r.decoder = admission.NewDecoder(mgr.GetScheme())
}

func (r *LabelIndexerWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	var obj metav1.PartialObjectMetadata

	if err := r.decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	origMarshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var unstructuredObj map[string]any
	if err = json.Unmarshal(req.Object.Raw, &unstructuredObj); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	for _, objectRules := range r.indexingRules[obj.GetObjectKind().GroupVersionKind().Kind] {
		var labels map[string]string
		labels, err = objectRules.Apply(unstructuredObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		for k, v := range labels {
			obj.SetLabels(tools.SetMapValue(obj.GetLabels(), k, v))
		}
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
