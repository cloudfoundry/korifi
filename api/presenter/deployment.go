package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
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
	GUID          string                             `json:"guid"`
	Status        DeploymentStatus                   `json:"status"`
	Droplet       DropletGUID                        `json:"droplet"`
	Relationships map[string]model.ToOneRelationship `json:"relationships"`
	Links         DeploymentLinks                    `json:"links"`
	CreatedAt     string                             `json:"created_at"`
	UpdatedAt     string                             `json:"updated_at"`
}

type DeploymentLinks struct {
	Self Link `json:"self"`
	App  Link `json:"app"`
}

func ForDeployment(responseDeployment repositories.DeploymentRecord, baseURL url.URL) DeploymentResponse {
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
		CreatedAt:     formatTimestamp(&responseDeployment.CreatedAt),
		UpdatedAt:     formatTimestamp(responseDeployment.UpdatedAt),
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
