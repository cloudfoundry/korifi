package model

import (
	"encoding/json"
	"fmt"
)

type IncludedResource struct {
	Type     string
	Resource any
}

func (r IncludedResource) SelectJSONFields(fields ...string) (IncludedResource, error) {
	resourceBytes, err := json.Marshal(r.Resource)
	if err != nil {
		return IncludedResource{}, fmt.Errorf("failed to marshal resource: %w", err)
	}

	resourceMap := map[string]any{}
	if err := json.Unmarshal(resourceBytes, &resourceMap); err != nil {
		return IncludedResource{}, fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	if len(fields) == 0 {
		return IncludedResource{
			Type:     r.Type,
			Resource: resourceMap,
		}, nil
	}

	resourceFromFields := map[string]any{}
	for _, field := range fields {
		resourceFromFields[field] = resourceMap[field]
	}

	return IncludedResource{
		Type:     r.Type,
		Resource: resourceFromFields,
	}, nil
}
