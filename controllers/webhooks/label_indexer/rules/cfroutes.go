package rules

import (
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values"
	"github.com/PaesslerAG/jsonpath"
)

func DestinationAppGuidLabelRules(obj map[string]any) ([]LabelRule, error) {
	destinations, err := jsonpath.Get("$.spec.destinations[*]", obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get route destinations: %w", err)
	}

	rules := []LabelRule{}

	for _, destination := range destinations.([]any) {
		appGuid, err := jsonpath.Get("$.appRef.name", destination)
		if err != nil {
			return nil, fmt.Errorf("failed to get destination appref name: %w", err)
		}

		if appGuid == "" {
			continue
		}

		rules = append(rules, LabelRule{
			Label:        korifiv1alpha1.DestinationAppGUIDLabelPrefix + appGuid.(string),
			IndexingFunc: values.EmptyValue(),
		})
	}

	return rules, nil
}

func RouteIsUnmappedLabelRule(obj map[string]any) ([]LabelRule, error) {
	destinations, err := jsonpath.Get("$.spec.destinations[*]", obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get route destinations: %w", err)
	}

	rules := []LabelRule{}

	rules = append(rules, LabelRule{
		Label:        korifiv1alpha1.CFRouteIsUnmappedLabelKey,
		IndexingFunc: values.DestinationsAreEmpty(destinations.([]any)),
	})

	return rules, nil
}
