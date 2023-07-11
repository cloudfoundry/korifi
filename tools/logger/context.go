package logger

import (
	"context"

	"github.com/go-logr/logr"
)

func FromContext(ctx context.Context, loggerName string) (context.Context, logr.Logger) {
	log := logr.FromContextOrDiscard(ctx).WithName(loggerName)
	return logr.NewContext(ctx, log), log
}
