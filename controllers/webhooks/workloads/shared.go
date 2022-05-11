package workloads

import workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name PlacementValidator . PlacementValidator

type PlacementValidator interface {
	ValidateOrgCreate(org workloadsv1alpha1.CFOrg) error
	ValidateSpaceCreate(space workloadsv1alpha1.CFSpace) error
}
