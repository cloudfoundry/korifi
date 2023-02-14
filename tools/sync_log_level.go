package tools

import (
	"context"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name LogLevelGetter . LogLevelGetter

type LogLevelGetter func(string) (zapcore.Level, error)

func SyncLogLevel(ctx context.Context, logger logr.Logger, eventChan chan string, atomicLevel zap.AtomicLevel, getLogLevelFromPath LogLevelGetter) {
	for {
		select {
		case configFilePath := <-eventChan:
			cfgLogLevel, err := getLogLevelFromPath(configFilePath)
			if err != nil {
				logger.Error(err, "error reading config")
				continue
			}

			if atomicLevel.Level() != cfgLogLevel {
				logger.Info("Updating logging level", "originalLevel", atomicLevel.Level(), "newLevel", cfgLogLevel)
				atomicLevel.SetLevel(cfgLogLevel)
			}

		case <-ctx.Done():
			logger.Info("Stopping change log level function")
			return
		}
	}
}
