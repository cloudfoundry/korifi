package actions

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	FetchProcess(context.Context, client.Client, string) (repositories.ProcessRecord, error)
	FetchProcessesForApp(context.Context, client.Client, string, string) ([]repositories.ProcessRecord, error)
	ScaleProcess(context.Context, client.Client, repositories.ScaleProcessMessage) (repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	FetchApp(context.Context, client.Client, string) (repositories.AppRecord, error)
	FetchAppList(context.Context, client.Client) ([]repositories.AppRecord, error)
	FetchNamespace(context.Context, client.Client, string) (repositories.SpaceRecord, error)
	CreateAppEnvironmentVariables(context.Context, client.Client, repositories.AppEnvVarsRecord) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, client.Client, repositories.AppRecord) (repositories.AppRecord, error)
	SetCurrentDroplet(context.Context, client.Client, repositories.SetCurrentDropletMessage) (repositories.CurrentDropletRecord, error)
	SetAppDesiredState(context.Context, client.Client, repositories.SetAppDesiredStateMessage) (repositories.AppRecord, error)
}
