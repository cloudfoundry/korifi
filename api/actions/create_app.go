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
	// TODO: this action is so simple that we may want to inline it
	appCreateMessage := payload.ToAppCreateMessage()

	return a.appRepo.CreateApp(ctx, c, appCreateMessage)
}
