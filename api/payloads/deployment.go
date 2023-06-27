package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
)

type DropletGUID struct {
	Guid string `json:"guid"`
}

type DeploymentCreate struct {
	Droplet       DropletGUID              `json:"droplet"`
	Relationships *DeploymentRelationships `json:"relationships" validate:"required"`
}

type DeploymentRelationships struct {
	App *Relationship `json:"app" validate:"required"`
}

func (c *DeploymentCreate) ToMessage() repositories.CreateDeploymentMessage {
	return repositories.CreateDeploymentMessage{
		AppGUID:     c.Relationships.App.Data.GUID,
		DropletGUID: c.Droplet.Guid,
	}
}
