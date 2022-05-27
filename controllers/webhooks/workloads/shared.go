package workloads

import korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name PlacementValidator . PlacementValidator

type PlacementValidator interface {
	ValidateOrgCreate(org korifiv1alpha1.CFOrg) error
	ValidateSpaceCreate(space korifiv1alpha1.CFSpace) error
}
