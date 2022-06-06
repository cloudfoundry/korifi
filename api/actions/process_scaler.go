package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ProcessScaler struct {
	appRepo     CFAppRepository
	processRepo CFProcessRepository
}

func NewProcessScaler(appRepo CFAppRepository, processRepo CFProcessRepository) *ProcessScaler {
	return &ProcessScaler{
		appRepo:     appRepo,
		processRepo: processRepo,
	}
}

func (a *ProcessScaler) ScaleAppProcess(ctx context.Context, authInfo authorization.Info, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
	app, err := a.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return repositories.ProcessRecord{}, apierrors.ForbiddenAsNotFound(err)
	}

	fetchProcessMessage := repositories.ListProcessesMessage{
		AppGUIDs:  []string{app.GUID},
		SpaceGUID: app.SpaceGUID,
	}

	appProcesses, err := a.processRepo.ListProcesses(ctx, authInfo, fetchProcessMessage)
	if err != nil {
		return repositories.ProcessRecord{}, apierrors.ForbiddenAsNotFound(err)
	}

	found := false
	var appProcess repositories.ProcessRecord
	for _, v := range appProcesses {
		if v.Type == processType {
			appProcess = v
			found = true
			break
		}
	}

	if !found {
		return repositories.ProcessRecord{}, apierrors.NewNotFoundError(nil, repositories.ProcessResourceType)
	}

	return a.scaleProcess(ctx, authInfo, appProcess, scale)
}

func (a *ProcessScaler) ScaleProcess(ctx context.Context, authInfo authorization.Info, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
	process, err := a.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		return repositories.ProcessRecord{}, apierrors.ForbiddenAsNotFound(err)
	}
	return a.scaleProcess(ctx, authInfo, process, scale)
}

func (a *ProcessScaler) scaleProcess(ctx context.Context, authInfo authorization.Info, process repositories.ProcessRecord, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
	scaleMessage := repositories.ScaleProcessMessage{
		GUID:               process.GUID,
		SpaceGUID:          process.SpaceGUID,
		ProcessScaleValues: scale,
	}
	return a.processRepo.ScaleProcess(ctx, authInfo, scaleMessage)
}
