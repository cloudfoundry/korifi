package actions

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
)

//counterfeiter:generate -o fake -fake-name StateCollector . StateCollector
type StateCollector interface {
	CollectState(ctx context.Context, authInfo authorization.Info, appName, spaceGUID string) (manifest.AppState, error)
}

//counterfeiter:generate -o fake -fake-name Normalizer . Normalizer
type Normalizer interface {
	Normalize(appInfo payloads.ManifestApplication, appState manifest.AppState) payloads.ManifestApplication
}

//counterfeiter:generate -o fake -fake-name Applier . Applier
type Applier interface {
	Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication, appState manifest.AppState) error
}

type Manifest struct {
	domainRepo        shared.CFDomainRepository
	defaultDomainName string
	stateCollector    StateCollector
	normalizer        Normalizer
	applier           Applier
}

func NewManifest(domainRepo shared.CFDomainRepository, defaultDomainName string, stateCollector StateCollector, normalizer Normalizer, applier Applier,
) *Manifest {
	return &Manifest{
		domainRepo:        domainRepo,
		defaultDomainName: defaultDomainName,
		stateCollector:    stateCollector,
		normalizer:        normalizer,
		applier:           applier,
	}
}

func (a *Manifest) Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, manifest payloads.Manifest) error {
	err := a.ensureDefaultDomainConfigured(ctx, authInfo)
	if err != nil {
		return err
	}

	appInfo := manifest.Applications[0]
	appState, err := a.stateCollector.CollectState(ctx, authInfo, appInfo.Name, spaceGUID)
	if err != nil {
		return err
	}
	appInfo = a.normalizer.Normalize(appInfo, appState)

	return a.applier.Apply(ctx, authInfo, spaceGUID, appInfo, appState)
}

func (a *Manifest) ensureDefaultDomainConfigured(ctx context.Context, authInfo authorization.Info) error {
	_, err := a.domainRepo.GetDomainByName(ctx, authInfo, a.defaultDomainName)
	if err != nil {
		return apierrors.AsUnprocessableEntity(
			err,
			fmt.Sprintf("The configured default domain %q was not found", a.defaultDomainName),
			apierrors.NotFoundError{},
		)
	}

	return nil
}
