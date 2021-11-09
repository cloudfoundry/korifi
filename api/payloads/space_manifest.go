package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

type SpaceManifestApply struct {
	Version      int                             `yaml:"version"`
	Applications []SpaceManifestApplyApplication `yaml:"applications" validate:"max=1,dive"`
}

type SpaceManifestApplyApplication struct {
	Name string            `yaml:"name" validate:"required"`
	Env  map[string]string `yaml:"env"`
}

func (a SpaceManifestApply) ToAppCreateMessage(spaceGUID string) repositories.AppCreateMessage {
	return repositories.AppCreateMessage{
		Name:      a.Applications[0].Name,
		SpaceGUID: spaceGUID,
		Lifecycle: repositories.Lifecycle{
			Type: string(v1alpha1.BuildpackLifecycle),
		},
		State: repositories.DesiredState(v1alpha1.StoppedState),
	}
}
