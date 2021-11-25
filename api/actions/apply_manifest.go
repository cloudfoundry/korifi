package actions

import (
	"context"
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type applyManifest struct {
	appRepo     CFAppRepository
	processRepo CFProcessRepository
}

func NewApplyManifest(appRepo CFAppRepository, processRepo CFProcessRepository) *applyManifest {
	return &applyManifest{
		appRepo:     appRepo,
		processRepo: processRepo,
	}
}

func (a *applyManifest) Invoke(ctx context.Context, c client.Client, spaceGUID string, manifest payloads.Manifest) error {
	appInfo := manifest.Applications[0]
	exists := true
	appRecord, err := a.appRepo.FetchAppByNameAndSpace(ctx, c, appInfo.Name, spaceGUID)
	if err != nil {
		if !errors.As(err, new(repositories.NotFoundError)) {
			return err
		}
		exists = false
	}

	if exists {
		return a.updateApp(ctx, c, spaceGUID, appRecord, appInfo)
	} else {
		return a.createApp(ctx, c, spaceGUID, appInfo)
	}
}

func (a *applyManifest) updateApp(ctx context.Context, c client.Client, spaceGUID string, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication) error {
	_, err := a.appRepo.CreateOrPatchAppEnvVars(ctx, c, repositories.CreateOrPatchAppEnvVarsMessage{
		AppGUID:              appRecord.GUID,
		AppEtcdUID:           appRecord.EtcdUID,
		SpaceGUID:            appRecord.SpaceGUID,
		EnvironmentVariables: appInfo.Env,
	})
	if err != nil {
		return err
	}

	for _, processInfo := range appInfo.Processes {
		exists := true

		var process repositories.ProcessRecord
		process, err = a.processRepo.FetchProcessByAppTypeAndSpace(ctx, c, appRecord.GUID, processInfo.Type, spaceGUID)
		if err != nil {
			if errors.As(err, new(repositories.NotFoundError)) {
				exists = false
			} else {
				return err
			}
		}

		if exists {
			err = a.processRepo.PatchProcess(ctx, c, processInfo.ToProcessPatchMessage(process.GUID, spaceGUID))
		} else {
			err = a.processRepo.CreateProcess(ctx, c, processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID))
		}
		if err != nil {
			return err
		}
	}

	return err
}

func (a *applyManifest) createApp(ctx context.Context, c client.Client, spaceGUID string, appInfo payloads.ManifestApplication) error {
	appRecord, err := a.appRepo.CreateApp(ctx, c, appInfo.ToAppCreateMessage(spaceGUID))
	if err != nil {
		return err
	}

	for _, processInfo := range appInfo.Processes {
		message := processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID)
		err = a.processRepo.CreateProcess(ctx, c, message)
		if err != nil {
			return err
		}
	}

	return nil
}
