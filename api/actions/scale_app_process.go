package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name ScaleProcess . ScaleProcessAction
type ScaleProcessAction func(ctx context.Context, client client.Client, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)

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

func (a *ScaleAppProcess) Invoke(ctx context.Context, client client.Client, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error) {
	app, err := a.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		return repositories.ProcessRecord{}, err
	}

	fetchProcessMessage := repositories.FetchProcessListMessage{
		AppGUID:   []string{app.GUID},
		SpaceGUID: app.SpaceGUID,
	}

	appProcesses, err := a.processRepo.FetchProcessList(ctx, client, fetchProcessMessage)
	if err != nil {
		return repositories.ProcessRecord{}, err
	}

	var appProcessGUID string
	for _, v := range appProcesses {
		if v.Type == processType {
			appProcessGUID = v.GUID
			break
		}
	}
	return a.scaleProcessAction(ctx, client, appProcessGUID, scale)
}
