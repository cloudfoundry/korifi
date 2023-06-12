package payloads

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jellydator/validation"
)

type BuildMetadata struct {
	Annotations map[string]string `json:"annotations" validate:"buildmetadatavalidator"`
	Labels      map[string]string `json:"labels" validate:"buildmetadatavalidator"`
}

type Metadata struct {
	Annotations map[string]string `json:"annotations" yaml:"annotations" validate:"metadatavalidator"`
	Labels      map[string]string `json:"labels"      yaml:"labels"      validate:"metadatavalidator"`
}

func (m Metadata) Validate() error {
	return validation.ValidateStruct(&m,
		validation.Field(&m.Annotations, validation.Map().Keys(validation.By(cloudfoundryKeyCheck)).AllowExtraKeys()),
		validation.Field(&m.Labels, validation.Map().Keys(validation.By(cloudfoundryKeyCheck)).AllowExtraKeys()),
	)
}

type MetadataPatch struct {
	Annotations map[string]*string `json:"annotations" validate:"metadatavalidator"`
	Labels      map[string]*string `json:"labels"      validate:"metadatavalidator"`
}

func (p MetadataPatch) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Annotations, validation.Map().Keys(validation.By(cloudfoundryKeyCheck)).AllowExtraKeys()),
		validation.Field(&p.Labels, validation.Map().Keys(validation.By(cloudfoundryKeyCheck)).AllowExtraKeys()),
	)
}

func cloudfoundryKeyCheck(key any) error {
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("expected string key, got %T", key)
	}

	u, err := url.ParseRequestURI("https://" + keyStr) // without the scheme, the hostname will be parsed as a path
	if err != nil {
		return nil
	}

	if strings.HasSuffix(u.Hostname(), "cloudfoundry.org") {
		return errors.New("label/annotation key cannot use the cloudfoundry.org domain")
	}
	return nil
}
