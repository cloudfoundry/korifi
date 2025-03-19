package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
)

const securityGroupBase = "/v3/security_groups"

type SecurityGroupResponse struct {
	GUID            string                                `json:"guid"`
	CreatedAt       string                                `json:"created_at"`
	UpdatedAt       string                                `json:"updated_at"`
	Name            string                                `json:"name"`
	GloballyEnabled korifiv1alpha1.SecurityGroupWorkloads `json:"globally_enabled"`
	Rules           []korifiv1alpha1.SecurityGroupRule    `json:"rules"`
	Relationships   payloads.SecurityGroupRelationships   `json:"relationships"`
	Links           SecurityGroupLinks                    `json:"links"`
}

type SecurityGroupLinks struct {
	Self Link `json:"self"`
}

func ForSecurityGroup(securityGroupRecord repositories.SecurityGroupRecord, baseURL url.URL, includes ...include.Resource) SecurityGroupResponse {
	return SecurityGroupResponse{
		GUID:            securityGroupRecord.GUID,
		CreatedAt:       tools.ZeroIfNil(formatTimestamp(&securityGroupRecord.CreatedAt)),
		UpdatedAt:       tools.ZeroIfNil(formatTimestamp(securityGroupRecord.UpdatedAt)),
		Name:            securityGroupRecord.Name,
		GloballyEnabled: securityGroupRecord.GloballyEnabled,
		Rules:           securityGroupRecord.Rules,
		Relationships: payloads.SecurityGroupRelationships{
			RunningSpaces: payloads.ToManyRelationship{
				Data: toManyRelationshipData(securityGroupRecord.RunningSpaces),
			},
			StagingSpaces: payloads.ToManyRelationship{
				Data: toManyRelationshipData(securityGroupRecord.StagingSpaces),
			},
		},
		Links: SecurityGroupLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(securityGroupBase, securityGroupRecord.GUID).build(),
			},
		},
	}
}

func toManyRelationshipData(guids []string) []payloads.RelationshipData {
	if guids == nil {
		return []payloads.RelationshipData{}
	}
	data := make([]payloads.RelationshipData, 0, len(guids))
	for _, guid := range guids {
		data = append(data, payloads.RelationshipData{GUID: guid})
	}
	return data
}
