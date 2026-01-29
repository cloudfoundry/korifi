package presenter

import (
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	deploymentsBase = "/v3/deployments"
)

type DeploymentStatus struct {
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type DropletGUID struct {
	Guid string `json:"guid"`
}
type DeploymentResponse struct {
	GUID          string                       `json:"guid"`
	Status        DeploymentStatus             `json:"status"`
	Droplet       DropletGUID                  `json:"droplet"`
	Relationships map[string]ToOneRelationship `json:"relationships"`
	Links         DeploymentLinks              `json:"links"`
	CreatedAt     time.Time                    `json:"created_at"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

type DeploymentLinks struct {
	Self Link `json:"self"`
	App  Link `json:"app"`
}

func ForDeployment(responseDeployment repositories.DeploymentRecord, baseURL url.URL, includes ...include.Resource) DeploymentResponse {
	return DeploymentResponse{
		GUID: responseDeployment.GUID,
		Status: DeploymentStatus{
			Value:  string(responseDeployment.Status.Value),
			Reason: string(responseDeployment.Status.Reason),
		},
		Droplet: DropletGUID{
			Guid: responseDeployment.DropletGUID,
		},
		Relationships: ForRelationships(responseDeployment.Relationships()),
		CreatedAt:     tools.ZeroIfNil(toUTC(&responseDeployment.CreatedAt)),
		UpdatedAt:     tools.ZeroIfNil(toUTC(responseDeployment.UpdatedAt)),
		Links: DeploymentLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(deploymentsBase, responseDeployment.GUID).build(),
			},
			App: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseDeployment.GUID).build(),
			},
		},
	}
}
