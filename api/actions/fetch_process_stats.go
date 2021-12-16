package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
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
	processRecord, err := a.processRepo.FetchProcess(ctx, authInfo, processGUID)
	if err != nil {
		return nil, err
	}
	appRecord, err := a.appRepo.FetchApp(ctx, authInfo, processRecord.AppGUID)
	if err != nil {
		return nil, err
	}

	if appRecord.State == repositories.StoppedState {
		return []repositories.PodStatsRecord{}, nil
	}

	message := repositories.FetchPodStatsMessage{
		Namespace:   processRecord.SpaceGUID,
		AppGUID:     processRecord.AppGUID,
		Instances:   processRecord.DesiredInstances,
		ProcessType: processRecord.Type,
		AppRevision: appRecord.Revision,
	}
	return a.podRepo.FetchPodStatsByAppGUID(ctx, authInfo, message)
}
