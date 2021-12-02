package actions

import (
	"context"
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
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

func (a *applyManifest) Invoke(ctx context.Context, authInfo authorization.Info, spaceGUID string, manifest payloads.Manifest) error {
	appInfo := manifest.Applications[0]
	exists := true
	appRecord, err := a.appRepo.FetchAppByNameAndSpace(ctx, authInfo, appInfo.Name, spaceGUID)
	if err != nil {
		if !errors.As(err, new(repositories.NotFoundError)) {
			return err
		}
		exists = false
	}

	if exists {
		return a.updateApp(ctx, authInfo, spaceGUID, appRecord, appInfo)
	} else {
		return a.createApp(ctx, authInfo, spaceGUID, appInfo)
	}
}

func (a *applyManifest) updateApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication) error {
	_, err := a.appRepo.CreateOrPatchAppEnvVars(ctx, authInfo, repositories.CreateOrPatchAppEnvVarsMessage{
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
		process, err = a.processRepo.FetchProcessByAppTypeAndSpace(ctx, authInfo, appRecord.GUID, processInfo.Type, spaceGUID)
		if err != nil {
			if errors.As(err, new(repositories.NotFoundError)) {
				exists = false
			} else {
				return err
			}
		}

		if exists {
			err = a.processRepo.PatchProcess(ctx, authInfo, processInfo.ToProcessPatchMessage(process.GUID, spaceGUID))
		} else {
			err = a.processRepo.CreateProcess(ctx, authInfo, processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID))
		}
		if err != nil {
			return err
		}
	}

	return err
}

func (a *applyManifest) createApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication) error {
	appRecord, err := a.appRepo.CreateApp(ctx, authInfo, appInfo.ToAppCreateMessage(spaceGUID))
	if err != nil {
		return err
	}

	for _, processInfo := range appInfo.Processes {
		message := processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID)
		err = a.processRepo.CreateProcess(ctx, authInfo, message)
		if err != nil {
			return err
		}
	}

	return nil
}
