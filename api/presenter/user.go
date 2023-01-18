package presenter

type UserResponse struct {
	Name string `json:"username"`
}

func ForUser(name string) UserResponse {
	return UserResponse{Name: name}
}
