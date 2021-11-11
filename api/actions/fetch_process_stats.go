package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (a *FetchProcessStats) Invoke(ctx context.Context, c client.Client, processGUID string) ([]repositories.PodStatsRecord, error) {
	processRecord, err := a.processRepo.FetchProcess(ctx, c, processGUID)
	if err != nil {
		return nil, err
	}
	appRecord, err := a.appRepo.FetchApp(ctx, c, processRecord.AppGUID)
	if err != nil {
		return nil, err
	}

	if appRecord.State == repositories.StoppedState {
		return []repositories.PodStatsRecord{}, nil
	}

	message := repositories.FetchPodStatsMessage{
		Namespace:   processRecord.SpaceGUID,
		AppGUID:     processRecord.AppGUID,
		Instances:   processRecord.Instances,
		ProcessType: processRecord.Type,
	}
	return a.podRepo.FetchPodStatsByAppGUID(ctx, c, message)
}
