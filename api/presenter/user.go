package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories/include"
)

const usersBase = "/v3/users"

type UserResponse struct {
	Name string `json:"username"`
	GUID string `json:"guid"`
}

func ForUser(name string, _ url.URL, includes ...include.Resource) UserResponse {
	return UserResponse{Name: name, GUID: name}
}
