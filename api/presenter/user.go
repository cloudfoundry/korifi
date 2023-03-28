package presenter

import "net/url"

const usersBase = "/v3/users"

type UserResponse struct {
	Name string `json:"username"`
	GUID string `json:"guid"`
}

func ForUser(name string, _ url.URL) UserResponse {
	return UserResponse{Name: name, GUID: name}
}
