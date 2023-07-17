package shared

import (
	"context"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-logr/logr"
)

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository

type CFProcessRepository interface {
	GetProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
	ListProcesses(context.Context, authorization.Info, repositories.ListProcessesMessage) ([]repositories.ProcessRecord, error)
	ScaleProcess(context.Context, authorization.Info, repositories.ScaleProcessMessage) (repositories.ProcessRecord, error)
	CreateProcess(context.Context, authorization.Info, repositories.CreateProcessMessage) error
	PatchProcess(context.Context, authorization.Info, repositories.PatchProcessMessage) (repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository

type CFAppRepository interface {
	GetApp(context.Context, authorization.Info, string) (repositories.AppRecord, error)
	GetAppByNameAndSpace(context.Context, authorization.Info, string, string) (repositories.AppRecord, error)
	CreateOrPatchAppEnvVars(context.Context, authorization.Info, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.CreateAppMessage) (repositories.AppRecord, error)
	PatchApp(context.Context, authorization.Info, repositories.PatchAppMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFBuildRepository . CFBuildRepository

type CFBuildRepository interface {
	GetLatestBuildByAppGUID(context.Context, authorization.Info, string, string) (repositories.BuildRecord, error)
	GetBuildLogs(context.Context, authorization.Info, string, string) ([]repositories.LogRecord, error)
}

//counterfeiter:generate -o fake -fake-name PodRepository . PodRepository

type PodRepository interface {
	GetRuntimeLogsForApp(context.Context, logr.Logger, authorization.Info, repositories.RuntimeLogsMessage) ([]repositories.LogRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	GetDomainByName(context.Context, authorization.Info, string) (repositories.DomainRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	GetOrCreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	ListRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsToRouteMessage) (repositories.RouteRecord, error)
	RemoveDestinationFromRoute(ctx context.Context, authInfo authorization.Info, message repositories.RemoveDestinationFromRouteMessage) (repositories.RouteRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFServiceBindingRepository . CFServiceBindingRepository
type CFServiceBindingRepository interface {
	CreateServiceBinding(context.Context, authorization.Info, repositories.CreateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
	DeleteServiceBinding(context.Context, authorization.Info, string) error
	ListServiceBindings(context.Context, authorization.Info, repositories.ListServiceBindingsMessage) ([]repositories.ServiceBindingRecord, error)
	UpdateServiceBinding(context.Context, authorization.Info, repositories.UpdateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFServiceInstanceRepository . CFServiceInstanceRepository
type CFServiceInstanceRepository interface {
	ListServiceInstances(context.Context, authorization.Info, repositories.ListServiceInstanceMessage) ([]repositories.ServiceInstanceRecord, error)
}
