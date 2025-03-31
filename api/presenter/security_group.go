package presenter

import (
	"net/url"
	"slices"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
)

const securityGroupBase = "/v3/security_groups"

type SecurityGroupResponse struct {
	GUID            string                              `json:"guid"`
	CreatedAt       string                              `json:"created_at"`
	UpdatedAt       string                              `json:"updated_at"`
	Name            string                              `json:"name"`
	GloballyEnabled repositories.SecurityGroupWorkloads `json:"globally_enabled"`
	Rules           []repositories.SecurityGroupRule    `json:"rules"`
	Relationships   payloads.SecurityGroupRelationships `json:"relationships"`
	Links           SecurityGroupLinks                  `json:"links"`
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
	if len(guids) == 0 {
		return []payloads.RelationshipData{}
	}

	return slices.Collect(it.Map(slices.Values(guids), func(guid string) payloads.RelationshipData {
		return payloads.RelationshipData{GUID: guid}
	}))
}
