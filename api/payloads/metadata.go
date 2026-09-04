package payloads

import (
	"code.cloudfoundry.org/korifi/api/tools/metadata"
	"github.com/jellydator/validation"
)

type BuildMetadata struct {
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
}

func (m BuildMetadata) Validate() error {
	return validation.ValidateStruct(&m,
		validation.Field(&m.Annotations, validation.Empty),
		validation.Field(&m.Labels, validation.Empty),
	)
}

type Metadata struct {
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
	Labels      map[string]string `json:"labels"      yaml:"labels"`
}

func (m Metadata) Validate() error {
	return validation.ValidateStruct(&m,
		validation.Field(&m.Annotations, validation.Map().Keys(validation.By(metadata.CloudfoundryKeyCheck)).AllowExtraKeys()),
		validation.Field(&m.Labels, validation.Map().Keys(validation.By(metadata.CloudfoundryKeyCheck)).AllowExtraKeys()),
	)
}

type MetadataPatch struct {
	Annotations map[string]*string `json:"annotations,omitempty"`
	Labels      map[string]*string `json:"labels,omitempty"`
}
