package authorization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"k8s.io/apiserver/pkg/authentication/user"
)

type Info struct {
	Token    string
	CertData []byte
	UserInfo user.Info
}

type key int

var infoKey key

func NewContext(ctx context.Context, info *Info) context.Context {
	return context.WithValue(ctx, infoKey, info)
}

func InfoFromContext(ctx context.Context) (Info, bool) {
	info, ok := ctx.Value(infoKey).(*Info)
	if info == nil {
		return Info{}, ok
	}

	return *info, ok
}

func (i Info) Scheme() string {
	if i.Token != "" {
		return BearerScheme
	}

	if len(i.CertData) > 0 {
		return CertScheme
	}

	return UnknownScheme
}

func (i Info) Hash() string {
	key := append([]byte(i.Token), i.CertData...)
	hasher := sha256.New()
	return hex.EncodeToString(hasher.Sum(key))
}
