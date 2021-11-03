package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	"github.com/google/uuid"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

type SpaceManifestApply struct {
	Version      int                             `yaml:"version"`
	Applications []SpaceManifestApplyApplication `yaml:"applications" validate:"max=1,dive"`
}

type SpaceManifestApplyApplication struct {
	Name string `yaml:"name" validate:"required"`
}

func (a SpaceManifestApply) ToRecord(spaceGUID string) repositories.AppRecord {
	appGUID := uuid.New().String()
	return repositories.AppRecord{
		GUID:      appGUID,
		Name:      a.Applications[0].Name,
		SpaceGUID: spaceGUID,
		Lifecycle: repositories.Lifecycle{
			Type: string(v1alpha1.BuildpackLifecycle),
		},
		State: repositories.DesiredState(v1alpha1.StoppedState),
	}
}
