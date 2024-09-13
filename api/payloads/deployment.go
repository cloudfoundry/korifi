package payloads

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/BooleanCat/go-functional/v2/it"
	jellidation "github.com/jellydator/validation"
)

type DropletGUID struct {
	Guid string `json:"guid"`
}

type DeploymentCreate struct {
	Droplet       DropletGUID              `json:"droplet"`
	Relationships *DeploymentRelationships `json:"relationships"`
}

func (c DeploymentCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Relationships, jellidation.NotNil))
}

func (c *DeploymentCreate) ToMessage() repositories.CreateDeploymentMessage {
	return repositories.CreateDeploymentMessage{
		AppGUID:     c.Relationships.App.Data.GUID,
		DropletGUID: c.Droplet.Guid,
	}
}

type DeploymentRelationships struct {
	App *Relationship `json:"app"`
}

func (r DeploymentRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.App, jellidation.NotNil))
}

type DeploymentList struct {
	AppGUIDs     string `json:"app_guids"`
	OrderBy      string `json:"order_by"`
	StatusValues string `json:"status_values"`
}

func (d *DeploymentList) SupportedKeys() []string {
	return []string{"app_guids", "status_values", "order_by"}
}

func (d *DeploymentList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile("page"),
		regexp.MustCompile("per_page"),
	}
}

func (d *DeploymentList) DecodeFromURLValues(values url.Values) error {
	d.AppGUIDs = values.Get("app_guids")
	d.OrderBy = values.Get("order_by")
	d.StatusValues = values.Get("status_values")

	return nil
}

func (d DeploymentList) Validate() error {
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&d.StatusValues, jellidation.By(func(value any) error {
			statusValues, ok := value.(string)
			if !ok {
				return fmt.Errorf("%T is not supported, string is expected", value)
			}

			return jellidation.Each(validation.OneOf(
				"ACTIVE",
				"FINALIZED",
			)).Validate(parse.ArrayParam(statusValues))
		})),
	)
}

func (d *DeploymentList) ToMessage() repositories.ListDeploymentsMessage {
	statusValues := slices.Collect(it.Map(slices.Values(parse.ArrayParam(d.StatusValues)), func(v string) repositories.DeploymentStatusValue {
		return repositories.DeploymentStatusValue(v)
	}))

	return repositories.ListDeploymentsMessage{
		AppGUIDs:     parse.ArrayParam(d.AppGUIDs),
		StatusValues: statusValues,
		OrderBy:      d.OrderBy,
	}
}
