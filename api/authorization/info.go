package authorization

import "context"

type Info struct {
	Token    string
	CertData []byte
}

type key int

var infoKey key

func NewContext(ctx context.Context, info *Info) context.Context {
	return context.WithValue(ctx, infoKey, info)
}

func InfoFromContext(ctx context.Context) (*Info, bool) {
	info, ok := ctx.Value(infoKey).(*Info)
	return info, ok
}
