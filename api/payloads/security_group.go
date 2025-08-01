package payloads

import (
	"net/url"
	"slices"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
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

type SecurityGroupList struct {
	Names                  string
	GUIDs                  string
	RunningSpaceGUIDs      string
	StagingSpaceGUIDs      string
	GloballyEnabledRunning bool
	GloballyEnabledStaging bool
	OrderBy                string
}

func (a SecurityGroupList) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
	)
}

func (a *SecurityGroupList) ToMessage() repositories.ListSecurityGroupMessage {
	return repositories.ListSecurityGroupMessage{
		Names:                  parse.ArrayParam(a.Names),
		GUIDs:                  parse.ArrayParam(a.GUIDs),
		RunningSpaceGUIDs:      parse.ArrayParam(a.RunningSpaceGUIDs),
		StagingSpaceGUIDs:      parse.ArrayParam(a.StagingSpaceGUIDs),
		GloballyEnabledRunning: a.GloballyEnabledRunning,
		GloballyEnabledStaging: a.GloballyEnabledStaging,
		OrderBy:                a.OrderBy,
	}
}

func (a *SecurityGroupList) SupportedKeys() []string {
	return []string{"names", "guids", "globally_enabled_running", "globally_enabled_staging", "running_space_guids", "staging_space_guids", "order_by", "per_page", "page"}
}

func (a *SecurityGroupList) DecodeFromURLValues(values url.Values) error {
	a.Names = values.Get("names")
	a.GUIDs = values.Get("guids")
	a.OrderBy = values.Get("order_by")
	a.RunningSpaceGUIDs = values.Get("running_space_guids")
	a.StagingSpaceGUIDs = values.Get("staging_space_guids")
	a.GloballyEnabledRunning = values.Get("globally_enabled_running") == "true"
	a.GloballyEnabledStaging = values.Get("globally_enabled_staging") == "true"
	return nil
}

type SecurityGroupBind struct {
	Data []RelationshipData `json:"data"`
}

func (b SecurityGroupBind) Validate() error {
	return jellidation.ValidateStruct(&b,
		jellidation.Field(&b.Data, jellidation.Required),
	)
}

func (b SecurityGroupBind) ToMessage(workload, guid string) repositories.BindSecurityGroupMessage {
	return repositories.BindSecurityGroupMessage{
		GUID: guid,
		Spaces: slices.Collect(it.Map(slices.Values(b.Data), func(v RelationshipData) string {
			return v.GUID
		})),
		Workload: workload,
	}
}
