package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type FetchProcessStats struct {
	processRepo CFProcessRepository
	podRepo     PodRepository
	appRepo     CFAppRepository
}

func NewFetchProcessStats(processRepo CFProcessRepository, podRepo PodRepository, appRepo CFAppRepository) *FetchProcessStats {
	return &FetchProcessStats{
		processRepo,
		podRepo,
		appRepo,
	}
}

func (a *FetchProcessStats) Invoke(ctx context.Context, authInfo authorization.Info, processGUID string) ([]repositories.PodStatsRecord, error) {
	processRecord, err := a.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		return nil, err
	}
	appRecord, err := a.appRepo.GetApp(ctx, authInfo, processRecord.AppGUID)
	if err != nil {
		return nil, err
	}

	if appRecord.State == repositories.StoppedState {
		return []repositories.PodStatsRecord{
			{
				Type:  processRecord.Type,
				Index: 0,
				State: "DOWN",
			},
		}, nil
	}

	message := repositories.ListPodStatsMessage{
		Namespace:   processRecord.SpaceGUID,
		AppGUID:     processRecord.AppGUID,
		AppRevision: appRecord.Revision,
		Instances:   processRecord.DesiredInstances,
		ProcessGUID: processRecord.GUID,
		ProcessType: processRecord.Type,
	}
	return a.podRepo.ListPodStats(ctx, authInfo, message)
}
