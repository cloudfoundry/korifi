package shared

import (
	"context"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
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
	ListApps(context.Context, authorization.Info, repositories.ListAppsMessage) ([]repositories.AppRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.CreateAppMessage) (repositories.AppRecord, error)
	PatchApp(context.Context, authorization.Info, repositories.PatchAppMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFDomainRepository . CFDomainRepository

type CFDomainRepository interface {
	ListDomains(context.Context, authorization.Info, repositories.ListDomainsMessage) ([]repositories.DomainRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFRouteRepository . CFRouteRepository

type CFRouteRepository interface {
	GetOrCreateRoute(context.Context, authorization.Info, repositories.CreateRouteMessage) (repositories.RouteRecord, error)
	ListRoutesForApp(context.Context, authorization.Info, string, string) ([]repositories.RouteRecord, error)
	AddDestinationsToRoute(ctx context.Context, c authorization.Info, message repositories.AddDestinationsMessage) (repositories.RouteRecord, error)
	RemoveDestinationFromRoute(ctx context.Context, authInfo authorization.Info, message repositories.RemoveDestinationMessage) (repositories.RouteRecord, error)
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
