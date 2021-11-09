package actions

import (
	"context"
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type applyManifest struct {
	appRepo   CFAppRepository
	createApp CreateAppFunc
}

//counterfeiter:generate -o fake -fake-name CreateAppFunc . CreateAppFunc
type CreateAppFunc func(ctx context.Context, client2 client.Client, create payloads.AppCreate) (repositories.AppRecord, error)

func NewApplyManifest(appRepo CFAppRepository, createApp CreateAppFunc) *applyManifest {
	return &applyManifest{
		appRepo:   appRepo,
		createApp: createApp,
	}
}

func (a *applyManifest) Invoke(ctx context.Context, c client.Client, spaceGUID string, manifest payloads.SpaceManifestApply) error {
	appInfo := manifest.Applications[0]
	exists := true
	appRecord, err := a.appRepo.FetchAppByNameAndSpace(ctx, c, appInfo.Name, spaceGUID)
	if err != nil {
		if !errors.As(err, new(repositories.NotFoundError)) {
			return err
		}
		exists = false
	}

	if !exists {
		appRecord, err = a.createApp(ctx, c, payloads.AppCreate{
			Name:                 appInfo.Name,
			EnvironmentVariables: appInfo.Env,
			Relationships: payloads.AppRelationships{
				Space: payloads.Relationship{
					Data: &payloads.RelationshipData{GUID: spaceGUID},
				},
			},
		})

		if err != nil {
			return err
		}
		return nil
	}

	_, err = a.appRepo.CreateOrPatchAppEnvVars(ctx, c, repositories.CreateOrPatchAppEnvVarsMessage{
		AppGUID:              appRecord.GUID,
		AppEtcdUID:           appRecord.EtcdUID,
		SpaceGUID:            appRecord.SpaceGUID,
		EnvironmentVariables: manifest.Applications[0].Env,
	})

	return err
}
