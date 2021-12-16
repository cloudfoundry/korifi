package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository

type CFProcessRepository interface {
	FetchProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
	FetchProcessList(context.Context, authorization.Info, repositories.FetchProcessListMessage) ([]repositories.ProcessRecord, error)
	ScaleProcess(context.Context, authorization.Info, repositories.ProcessScaleMessage) (repositories.ProcessRecord, error)
	CreateProcess(context.Context, authorization.Info, repositories.ProcessCreateMessage) error
	FetchProcessByAppTypeAndSpace(context.Context, authorization.Info, string, string, string) (repositories.ProcessRecord, error)
	PatchProcess(context.Context, authorization.Info, repositories.ProcessPatchMessage) error
}

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository

type CFAppRepository interface {
	FetchApp(context.Context, authorization.Info, string) (repositories.AppRecord, error)
	FetchAppByNameAndSpace(context.Context, authorization.Info, string, string) (repositories.AppRecord, error)
	FetchNamespace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	CreateOrPatchAppEnvVars(context.Context, authorization.Info, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.AppCreateMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name PodRepository . PodRepository

type PodRepository interface {
	FetchPodStatsByAppGUID(ctx context.Context, authInfo authorization.Info, message repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	FetchDomainByName(context.Context, authorization.Info, string) (repositories.DomainRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	FetchOrCreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	FetchRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsToRouteMessage) (repositories.RouteRecord, error)
}
