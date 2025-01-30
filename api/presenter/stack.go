package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	stacksBase = "/v3/stacks"
)

type StackResponse struct {
	GUID        string     `json:"guid"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Metadata    Metadata   `json:"metadata"`
	Links       StackLinks `json:"links"`
}

type StackLinks struct {
	Self Link `json:"self"`
}

func ForStack(stackRecord repositories.StackRecord, baseURL url.URL, includes ...model.IncludedResource) StackResponse {
	return StackResponse{
		GUID:      stackRecord.GUID,
		CreatedAt: tools.ZeroIfNil(formatTimestamp(&stackRecord.CreatedAt)),
		UpdatedAt: tools.ZeroIfNil(formatTimestamp(stackRecord.UpdatedAt)),
		Name:      stackRecord.Name,
		Links: StackLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(stacksBase, stackRecord.GUID).build(),
			},
		},
	}
}
