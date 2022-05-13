package correlation

import "context"

func ContextWithId(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}
