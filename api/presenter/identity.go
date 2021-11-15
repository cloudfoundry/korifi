package presenter

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
)

type IdentityResponse struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func ForWhoAmI(identity authorization.Identity) IdentityResponse {
	return IdentityResponse{
		Name: identity.Name,
		Kind: identity.Kind,
	}
}
