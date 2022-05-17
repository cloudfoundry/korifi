package correlation

import (
	"context"

	"github.com/go-logr/logr"
)

type key int

var correlationIDKey key

func getCorrelationID(ctx context.Context) (string, bool) {
	id := ctx.Value(correlationIDKey)
	s, ok := id.(string)

	return s, ok
}

func AddCorrelationIDToLogger(ctx context.Context, logger logr.Logger) logr.Logger {
	id, ok := getCorrelationID(ctx)
	if !ok {
		return logger
	}

	return logger.WithValues("correlation-id", id)
}
