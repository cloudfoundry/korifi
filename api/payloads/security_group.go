package payloads

import (
	"slices"

	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/BooleanCat/go-functional/v2/it"
	jellidation "github.com/jellydator/validation"
)

type SecurityGroupRule struct {
	Protocol    string `json:"protocol"`
	Destination string `json:"destination"`
	Ports       string `json:"ports,omitempty"`
	Type        int    `json:"type,omitempty"`
	Code        int    `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
	Log         bool   `json:"log,omitempty"`
}

type SecurityGroupWorkloads struct {
	Running bool `json:"running"`
	Staging bool `json:"staging"`
}

type SecurityGroupRelationships struct {
	RunningSpaces ToManyRelationship `json:"running_spaces"`
	StagingSpaces ToManyRelationship `json:"staging_spaces"`
}

type SecurityGroupCreate struct {
	DisplayName     string                     `json:"name"`
	Rules           []SecurityGroupRule        `json:"rules"`
	GloballyEnabled SecurityGroupWorkloads     `json:"globally_enabled"`
	Relationships   SecurityGroupRelationships `json:"relationships"`
}

func (c SecurityGroupCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.DisplayName, jellidation.Required),
		jellidation.Field(&c.Rules, jellidation.Required),
	)
}

func (c SecurityGroupCreate) ToMessage() repositories.CreateSecurityGroupMessage {
	rules := slices.Collect(it.Map(slices.Values(c.Rules), func(r SecurityGroupRule) repositories.SecurityGroupRule {
		return repositories.SecurityGroupRule{
			Protocol:    r.Protocol,
			Destination: r.Destination,
			Ports:       r.Ports,
			Type:        r.Type,
			Code:        r.Code,
			Description: r.Description,
			Log:         r.Log,
		}
	}))

	spaces := make(map[string]repositories.SecurityGroupWorkloads)
	runningSpaces := slices.Collect(it.Map(slices.Values(c.Relationships.RunningSpaces.Data), func(d RelationshipData) string { return d.GUID }))
	stagingSpaces := slices.Collect(it.Map(slices.Values(c.Relationships.StagingSpaces.Data), func(d RelationshipData) string { return d.GUID }))

	for _, guid := range runningSpaces {
		workloads := spaces[guid]
		workloads.Running = true
		spaces[guid] = workloads
	}

	for _, guid := range stagingSpaces {
		workloads := spaces[guid]
		workloads.Staging = true
		spaces[guid] = workloads
	}

	return repositories.CreateSecurityGroupMessage{
		DisplayName: c.DisplayName,
		Rules:       rules,
		GloballyEnabled: repositories.SecurityGroupWorkloads{
			Running: c.GloballyEnabled.Running,
			Staging: c.GloballyEnabled.Staging,
		},
		Spaces: spaces,
	}
}
