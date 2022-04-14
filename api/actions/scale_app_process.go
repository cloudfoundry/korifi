package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
)

//counterfeiter:generate -o fake -fake-name ScaleProcess . ScaleProcessAction
type ScaleProcessAction func(ctx context.Context, authInfo authorization.Info, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)

type ScaleAppProcess struct {
	appRepo            CFAppRepository
	processRepo        CFProcessRepository
	scaleProcessAction ScaleProcessAction
}

func NewScaleAppProcess(appRepo CFAppRepository, processRepo CFProcessRepository, scaleProcessAction ScaleProcessAction) *ScaleAppProcess {
	return &ScaleAppProcess{
		appRepo:            appRepo,
		processRepo:        processRepo,
		scaleProcessAction: scaleProcessAction,
	}
}

func (a *ScaleAppProcess) Invoke(ctx context.Context, authInfo authorization.Info, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
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

	var appProcessGUID string
	for _, v := range appProcesses {
		if v.Type == processType {
			appProcessGUID = v.GUID
			break
		}
	}

	if appProcessGUID == "" {
		return repositories.ProcessRecord{}, apierrors.NewNotFoundError(nil, repositories.ProcessResourceType)
	}
	return a.scaleProcessAction(ctx, authInfo, appProcessGUID, scale)
}
