package presenter

const usersBase = "/v3/users"

type UserResponse struct {
	Name string `json:"username"`
	GUID string `json:"guid"`
}

func ForUser(name string) UserResponse {
	return UserResponse{Name: name, GUID: name}
}
