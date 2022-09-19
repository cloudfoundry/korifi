package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ProcessStats struct {
	processRepo shared.CFProcessRepository
	podRepo     shared.PodRepository
	appRepo     shared.CFAppRepository
}

func NewProcessStats(processRepo shared.CFProcessRepository, podRepo shared.PodRepository, appRepo shared.CFAppRepository) *ProcessStats {
	return &ProcessStats{
		processRepo,
		podRepo,
		appRepo,
	}
}

func (a *ProcessStats) FetchStats(ctx context.Context, authInfo authorization.Info, processGUID string) ([]repositories.PodStatsRecord, error) {
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
