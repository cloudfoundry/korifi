package payloads

import (
	"net/url"
	"strconv"
	"strings"
)

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

func ParseArrayParam(arrayParam string) []string {
	if arrayParam == "" {
		return []string{}
	}

	elements := strings.Split(arrayParam, ",")
	for i, e := range elements {
		elements[i] = strings.TrimSpace(e)
	}

	return elements
}

type BuildMetadata struct {
	Annotations map[string]string `json:"annotations" validate:"buildmetadatavalidator"`
	Labels      map[string]string `json:"labels" validate:"buildmetadatavalidator"`
}

type Metadata struct {
	Annotations map[string]string `json:"annotations" yaml:"annotations" validate:"metadatavalidator"`
	Labels      map[string]string `json:"labels"      yaml:"labels"      validate:"metadatavalidator"`
}

type MetadataPatch struct {
	Annotations map[string]*string `json:"annotations" validate:"metadatavalidator"`
	Labels      map[string]*string `json:"labels"      validate:"metadatavalidator"`
}

func getInt(values url.Values, key string) (int64, error) {
	if !values.Has(key) {
		return 0, nil
	}
	s := values.Get(key)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func getBool(values url.Values, key string) (bool, error) {
	if !values.Has(key) {
		return false, nil
	}
	s := values.Get(key)
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}
