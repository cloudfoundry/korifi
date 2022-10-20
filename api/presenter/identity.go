package presenter

import (
	"code.cloudfoundry.org/korifi/api/authorization"
)

type IdentityResponse struct {
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
	Kind   string   `json:"kind"`
}

func ForWhoAmI(identity authorization.Identity) IdentityResponse {
	return IdentityResponse{
		Name:   identity.Name,
		Groups: identity.Groups,
		Kind:   identity.Kind,
	}
}
