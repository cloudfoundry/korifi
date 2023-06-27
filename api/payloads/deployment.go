package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type DropletGUID struct {
	Guid string `json:"guid"`
}

type DeploymentCreate struct {
	Droplet       DropletGUID              `json:"droplet"`
	Relationships *DeploymentRelationships `json:"relationships"`
}

func (c DeploymentCreate) Validate() error {
	return validation.ValidateStruct(&c,
		validation.Field(&c.Relationships, validation.NotNil))
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
	return validation.ValidateStruct(&r,
		validation.Field(&r.App, validation.NotNil))
}
