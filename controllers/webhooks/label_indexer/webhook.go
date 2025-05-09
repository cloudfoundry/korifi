package label_indexer

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-label-indexer,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=create;update,versions=v1alpha1,name=mcflabelindexer.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values"
	"code.cloudfoundry.org/korifi/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type IndexingRule struct {
	Label        string
	IndexingFunc IndexValueFunc
}

type LabelIndexerWebhook struct {
	decoder       admission.Decoder
	indexingRules map[string][]IndexingRule
}

func NewWebhook() *LabelIndexerWebhook {
	return &LabelIndexerWebhook{
		indexingRules: map[string][]IndexingRule{
			"CFRoute": {
				{Label: korifiv1alpha1.CFDomainGUIDLabelKey, IndexingFunc: Unquote(JSONValue("$.spec.domainRef.name"))},
				{Label: korifiv1alpha1.SpaceGUIDKey, IndexingFunc: Unquote(JSONValue("$.metadata.namespace"))},
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
	if err := json.Unmarshal(req.Object.Raw, &unstructuredObj); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	for _, rule := range r.indexingRules[obj.GetObjectKind().GroupVersionKind().Kind] {
		indexValue, err := rule.IndexingFunc(unstructuredObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if indexValue != nil {
			obj.SetLabels(tools.SetMapValue(obj.GetLabels(), rule.Label, *indexValue))
		}
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(origMarshalled, marshalled)
}
