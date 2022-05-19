package workloads

import "code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name PlacementValidator . PlacementValidator

type PlacementValidator interface {
	ValidateOrgCreate(org v1alpha1.CFOrg) error
	ValidateSpaceCreate(space v1alpha1.CFSpace) error
}
