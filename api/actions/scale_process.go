package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-api/repositories"

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

func (a *ScaleProcess) Invoke(ctx context.Context, client client.Client, processGUID string, scale repositories.ProcessScaleMessage) (repositories.ProcessRecord, error) {
	process, err := a.processRepo.FetchProcess(ctx, client, processGUID)
	if err != nil {
		return repositories.ProcessRecord{}, err
	}
	scaleMessage := repositories.ScaleProcessMessage{
		GUID:                process.GUID,
		SpaceGUID:           process.SpaceGUID,
		ProcessScaleMessage: scale,
	}
	return a.processRepo.ScaleProcess(ctx, client, scaleMessage)
}
