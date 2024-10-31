package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
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

func ForStack(stackRecord repositories.StackRecord, baseURL url.URL) StackResponse {
	return StackResponse{
		GUID:      stackRecord.GUID,
		CreatedAt: formatTimestamp(&stackRecord.CreatedAt),
		UpdatedAt: formatTimestamp(stackRecord.UpdatedAt),
		Name:      stackRecord.Name,
		Links: StackLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(stacksBase, stackRecord.GUID).build(),
			},
		},
	}
}
