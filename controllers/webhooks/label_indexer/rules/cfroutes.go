package rules

import (
	"fmt"
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/PaesslerAG/jsonpath"
)

func DestinationAppGuidLabelRules(obj map[string]any) ([]LabelRule, error) {
	appGUIDs, err := jsonpath.Get("$.spec.destinations[*].appRef.name", obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get route destinations: %w", err)
	}

	return slices.Collect(it.Map(it.Filter(slices.Values(appGUIDs.([]any)), func(appGuid any) bool {
		return appGuid.(string) != ""
	}), func(appGUID any) LabelRule {
		return LabelRule{
			Label:        korifiv1alpha1.DestinationAppGUIDLabelPrefix + appGUID.(string),
			IndexingFunc: values.EmptyValue(),
		}
	})), nil
}
