package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
)

const usersBase = "/v3/users"

type UserResponse struct {
	Name  string    `json:"username"`
	GUID  string    `json:"guid"`
	Links UserLinks `json:"links"`
}

type UserLinks struct {
	Self Link `json:"self"`
}

func ForUser(userRecord repositories.UserRecord, baseURL url.URL, _ ...include.Resource) UserResponse {
	return UserResponse{
		Name: userRecord.Name,
		GUID: userRecord.GUID,
		Links: UserLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(usersBase, userRecord.GUID).build(),
			},
		},
	}
}
