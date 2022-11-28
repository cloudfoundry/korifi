package payloads

import "strings"

type Lifecycle struct {
	Type string        `json:"type" validate:"required"`
	Data LifecycleData `json:"data" validate:"required"`
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks" validate:"required"`
	Stack      string   `json:"stack" validate:"required"`
}

type Relationship struct {
	Data *RelationshipData `json:"data" validate:"required"`
}

type RelationshipData struct {
	GUID string `json:"guid" validate:"required"`
}

func ParseArrayParam(arrayParam *string) []string {
	if arrayParam == nil {
		return []string{}
	}

	elements := strings.Split(*arrayParam, ",")
	for i, e := range elements {
		elements[i] = strings.TrimSpace(e)
	}

	return elements
}

type Metadata struct {
	Annotations map[string]string `json:"annotations" validate:"metadatavalidator"`
	Labels      map[string]string `json:"labels" validate:"metadatavalidator"`
}

type MetadataPatch struct {
	Annotations map[string]*string `json:"annotations" validate:"metadatavalidator"`
	Labels      map[string]*string `json:"labels" validate:"metadatavalidator"`
}
