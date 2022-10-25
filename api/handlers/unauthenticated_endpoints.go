package handlers

type UnauthenticatedEndpoints struct {
	unauthenticatedEndpoints map[string]interface{}
}

func NewUnauthenticatedEndpoints() *UnauthenticatedEndpoints {
	return &UnauthenticatedEndpoints{
		unauthenticatedEndpoints: map[string]interface{}{
			"/":            struct{}{},
			"/v3":          struct{}{},
			"/api/v1/info": struct{}{},
			"/oauth/token": struct{}{},
		},
	}
}

func (e *UnauthenticatedEndpoints) IsUnauthenticatedEndpoint(requestPath string) bool {
	_, authNotRequired := e.unauthenticatedEndpoints[requestPath]

	return authNotRequired
}
