package actions

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
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
	appRecord := manifest.ToRecord(spaceGUID)
	exists, err := a.appRepo.AppExistsWithNameAndSpace(ctx, c, appRecord.Name, appRecord.SpaceGUID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = a.appRepo.CreateApp(ctx, c, appRecord)
	return err
}
