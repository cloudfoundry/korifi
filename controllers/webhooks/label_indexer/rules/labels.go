package rules

import (
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
