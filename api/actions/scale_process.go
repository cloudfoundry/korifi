package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ScaleProcess struct {
	processRepo CFProcessRepository
}

func NewScaleProcess(processRepo CFProcessRepository) *ScaleProcess {
	return &ScaleProcess{
		processRepo: processRepo,
	}
}

func (a *ScaleProcess) Invoke(ctx context.Context, client client.Client, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
	process, err := a.processRepo.FetchProcess(ctx, client, processGUID)
	if err != nil {
		return repositories.ProcessRecord{}, err
	}
	scaleMessage := repositories.ProcessScaleMessage{
		GUID:               process.GUID,
		SpaceGUID:          process.SpaceGUID,
		ProcessScaleValues: scale,
	}
	return a.processRepo.ScaleProcess(ctx, client, scaleMessage)
}
