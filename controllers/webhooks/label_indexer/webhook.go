package label_indexer

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-label-indexer,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=create;update,versions=v1alpha1,name=mcflabelindexer.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/PaesslerAG/jsonpath"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type IndexingRule struct {
	Label        string
	IndexingFunc IndexingFunc
}

type LabelIndexerWebhook struct {
	decoder       admission.Decoder
	indexingRules map[string][]IndexingRule
}

type (
	IndexingFunc func(map[string]any) (*string, error)
)

func JSONValue(path string) IndexingFunc {
	return func(obj map[string]any) (*string, error) {
		value, err := jsonpath.Get(path, obj)
		if err != nil {
			if strings.HasPrefix(err.Error(), "unknown key") {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get value from JSONPath %s: %w", path, err)
		}

		return marshal(value)
	}
}

func SingleValue(prev IndexingFunc) IndexingFunc {
	return func(obj map[string]any) (*string, error) {
		jsonString, err := prev(obj)
		if err != nil {
			return nil, err
		}
		if jsonString == nil {
			return nil, nil
		}

		var array []any
		if err := json.Unmarshal([]byte(*jsonString), &array); err != nil {
			return nil, fmt.Errorf("failed to unmarshal value %s: %w", *jsonString, err)
		}

		if len(array) > 1 {
			return nil, fmt.Errorf("expected single value, got array %v", array)
		}

		if len(array) == 0 {
			return nil, nil
		}

		return marshal(array[0])
	}
}

func Unquote(prev IndexingFunc) IndexingFunc {
	return func(obj map[string]any) (*string, error) {
		prevValue, err := prev(obj)
		if err != nil {
			return nil, err
		}

		if prevValue == nil {
			return nil, nil
		}

		unquoted, err := strconv.Unquote(*prevValue)
		if err != nil {
			return nil, fmt.Errorf("failed to unquote value %s: %w", *prevValue, err)
		}

		return tools.PtrTo(unquoted), nil
	}
}

func SHA224(prev IndexingFunc) IndexingFunc {
	return func(obj map[string]any) (*string, error) {
		prevValue, err := prev(obj)
		if err != nil {
			return nil, err
		}
		if prevValue == nil {
			return nil, nil
		}

		return tools.PtrTo(tools.EncodeValueToSha224(*prevValue)), nil
	}
}

func marshal(value any) (*string, error) {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value %v: %w", value, err)
	}

	return tools.PtrTo(string(valueBytes)), nil
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
