package actions

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type createApp struct {
	appRepo CFAppRepository
}

func NewCreateApp(appRepo CFAppRepository) *createApp {
	return &createApp{
		appRepo: appRepo,
	}
}

func (a *createApp) Invoke(ctx context.Context, c client.Client, payload payloads.AppCreate) (repositories.AppRecord, error) {
	appCreateMessage := payload.ToAppCreateMessage()

	appRecord, err := a.appRepo.CreateApp(ctx, c, appCreateMessage)

	if err != nil {
		return repositories.AppRecord{}, err
	}

	envVarsMessage := repositories.CreateOrPatchAppEnvVarsMessage{
		AppGUID:              appRecord.GUID,
		AppEtcdUID:           appRecord.EtcdUID,
		SpaceGUID:            appRecord.SpaceGUID,
		EnvironmentVariables: payload.EnvironmentVariables,
	}
	_, err = a.appRepo.CreateOrPatchAppEnvVars(ctx, c, envVarsMessage)
	if err != nil {
		return repositories.AppRecord{}, err
	}

	return appRecord, nil
}
