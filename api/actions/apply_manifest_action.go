package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ApplyManifest struct {
	appRepo CFAppRepository
}

func NewApplyManifest(appRepo CFAppRepository) *ApplyManifest {
	return &ApplyManifest{
		appRepo: appRepo,
	}
}

func (a *ApplyManifest) Invoke(ctx context.Context, c client.Client, spaceGUID string, manifest payloads.SpaceManifestApply) error {
	appCreateMessage := manifest.ToAppCreateMessage(spaceGUID)
	exists, err := a.appRepo.AppExistsWithNameAndSpace(ctx, c, appCreateMessage.Name, appCreateMessage.SpaceGUID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	createdAppRecord, err := a.appRepo.CreateApp(ctx, c, appCreateMessage)
	if err != nil {
		return err
	}
	_, err = a.appRepo.CreateAppEnvironmentVariables(ctx, c, repositories.AppEnvVarsRecord{
		AppGUID:              createdAppRecord.GUID,
		SpaceGUID:            createdAppRecord.SpaceGUID,
		EnvironmentVariables: manifest.Applications[0].Env,
	})

	return nil
}
