package actions

import (
	"context"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-logr/logr"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository

type CFProcessRepository interface {
	GetProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
	ListProcesses(context.Context, authorization.Info, repositories.ListProcessesMessage) ([]repositories.ProcessRecord, error)
	ScaleProcess(context.Context, authorization.Info, repositories.ScaleProcessMessage) (repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository

type CFAppRepository interface {
	GetApp(context.Context, authorization.Info, string) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name CFBuildRepository . CFBuildRepository

type CFBuildRepository interface {
	GetLatestBuildByAppGUID(context.Context, authorization.Info, string, string) (repositories.BuildRecord, error)
	GetBuildLogs(context.Context, authorization.Info, string, string) ([]repositories.LogRecord, error)
}

//counterfeiter:generate -o fake -fake-name PodRepository . PodRepository

type PodRepository interface {
	ListPodStats(ctx context.Context, authInfo authorization.Info, message repositories.ListPodStatsMessage) ([]repositories.PodStatsRecord, error)
	GetRuntimeLogsForApp(context.Context, logr.Logger, authorization.Info, repositories.RuntimeLogsMessage) ([]repositories.LogRecord, error)
}
