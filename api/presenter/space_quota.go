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
	korifiv1alpha1.SpaceQuotaResource
	Links SpaceQuotaLinks `json:"links"`
}

type SpaceQuotaLinks struct {
	Self *Link `json:"self"`
}

func ForSpaceQuota(spaceQuotaResource korifiv1alpha1.SpaceQuotaResource, apiBaseURL url.URL) SpaceQuotaResponse {
	return SpaceQuotaResponse{
		spaceQuotaResource,
		SpaceQuotaLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(spaceQuotasBase, spaceQuotaResource.GUID).build(),
			},
		},
	}
}

func ForSpaceQuotaRelationships(guid string, relationsips korifiv1alpha1.ToManyRelationship, apiBaseURL url.URL) ToManyResponse {
	return ToManyResponse{
		relationsips,
		OrgQuotaRelationshipsLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(spaceQuotasBase, guid, "relationships", "spaces").build(),
			},
		},
	}
}
