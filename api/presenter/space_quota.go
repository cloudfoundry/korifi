package presenter

import (
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	// TODO: repetition with handler endpoint?
	spaceQuotasBase = "/v3/space_quotas"
)

type SpaceQuotaResponse struct {
	korifiv1alpha1.SpaceQuota

	//CreatedAt string `json:"created_at"`
	//UpdatedAt string `json:"updated_at"`

	Links SpaceQuotaLinks `json:"links"`
}

type SpaceQuotaLinks struct {
	Self *Link `json:"self"`
}

func ForSpaceQuota(spaceQuota korifiv1alpha1.SpaceQuota, apiBaseURL url.URL) SpaceQuotaResponse {
	return SpaceQuotaResponse{
		spaceQuota,
		SpaceQuotaLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(orgsBase, spaceQuota.GUID).build(),
			},
		},
	}
}
