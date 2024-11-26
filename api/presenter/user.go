package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/model"
)

const usersBase = "/v3/users"

type UserResponse struct {
	Name string `json:"username"`
	GUID string `json:"guid"`
}

func ForUser(name string, _ url.URL, includes ...model.IncludedResource) UserResponse {
	return UserResponse{Name: name, GUID: name}
}
