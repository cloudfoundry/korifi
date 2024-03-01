package presenter

import (
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	// TODO: repetition with handler endpoint?
	orgQuotasBase = "/v3/organization_quotas"
)

type OrgQuotaLinks struct {
	Self *Link `json:"self"`
}

type OrgQuotaResponse struct {
	korifiv1alpha1.OrgQuotaResource

	Links OrgQuotaLinks `json:"links"`
}

type OrgQuotaRelationshipsLinks struct {
	Self *Link `json:"self"`
}

type ToManyResponse struct {
	korifiv1alpha1.ToManyRelationship
	Links OrgQuotaRelationshipsLinks `json:"links"`
}

func ForOrgQuota(orgQuota korifiv1alpha1.OrgQuotaResource, apiBaseURL url.URL) OrgQuotaResponse {
	return OrgQuotaResponse{
		orgQuota,
		OrgQuotaLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(orgQuotasBase, orgQuota.GUID).build(),
			},
		},
	}
}

func ForOrgQuotaRelationships(guid string, relationsips korifiv1alpha1.ToManyRelationship, apiBaseURL url.URL) ToManyResponse {
	return ToManyResponse{
		relationsips,
		OrgQuotaRelationshipsLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(orgsBase, guid, "relationships", "organizations").build(),
			},
		},
	}
}
