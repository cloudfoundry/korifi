package rules

import (
	"maps"

	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values"
)

type LabelRule struct {
	Label        string
	IndexingFunc values.IndexValueFunc
}

func (r LabelRule) Apply(obj map[string]any) (map[string]string, error) {
	labelValue, err := r.IndexingFunc(obj)
	if err != nil {
		return nil, err
	}

	if labelValue == nil {
		return nil, nil
	}

	return map[string]string{
		r.Label: *labelValue,
	}, nil
}

type MultiLabelRule struct {
	LabelRules func(obj map[string]any) ([]LabelRule, error)
}

func (r MultiLabelRule) Apply(obj map[string]any) (map[string]string, error) {
	rules, err := r.LabelRules(obj)
	if err != nil {
		return nil, err
	}

	result := map[string]string{}

	for _, r := range rules {
		singleRuleLabels, err := r.Apply(obj)
		if err != nil {
			return nil, err
		}

		maps.Copy(result, singleRuleLabels)
	}

	return result, nil
}
