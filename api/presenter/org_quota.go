package presenter

import (
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	// TODO: repetition with handler endpoint?
	orgQuotasBase = "/v3/organization_quotas"
)

type OrgQuotaResponse struct {
	korifiv1alpha1.OrgQuota

	//CreatedAt string `json:"created_at"`
	//UpdatedAt string `json:"updated_at"`

	Links OrgQuotaLinks `json:"links"`
}

type OrgQuotaLinks struct {
	Self *Link `json:"self"`
}

func ForOrgQuota(orgQuota korifiv1alpha1.OrgQuota, apiBaseURL url.URL) OrgQuotaResponse {
	return OrgQuotaResponse{
		orgQuota,
		OrgQuotaLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(orgsBase, orgQuota.GUID).build(),
			},
		},
	}
}
