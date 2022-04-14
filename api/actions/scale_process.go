package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ScaleProcess struct {
	processRepo CFProcessRepository
}

func NewScaleProcess(processRepo CFProcessRepository) *ScaleProcess {
	return &ScaleProcess{
		processRepo: processRepo,
	}
}

func (a *ScaleProcess) Invoke(ctx context.Context, authInfo authorization.Info, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
	process, err := a.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		return repositories.ProcessRecord{}, apierrors.ForbiddenAsNotFound(err)
	}
	scaleMessage := repositories.ScaleProcessMessage{
		GUID:               process.GUID,
		SpaceGUID:          process.SpaceGUID,
		ProcessScaleValues: scale,
	}
	return a.processRepo.ScaleProcess(ctx, authInfo, scaleMessage)
}
