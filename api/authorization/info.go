package authorization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/golang-jwt/jwt"
)

type Info struct {
	Token    string
	CertData []byte
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

func (i Info) UserId() string {
	return i.getJwtClaim("user_id")
}

func (i Info) UserName() string {
	return i.getJwtClaim("user_name")
}

func (i Info) Email() string {
	return i.getJwtClaim("email")
}

func (i Info) getJwtClaim(claimName string) string {
	jwtToken, _, err := new(jwt.Parser).ParseUnverified(i.Token, jwt.MapClaims{})
	if err != nil {
		return ""
	}
	if claims, ok := jwtToken.Claims.(jwt.MapClaims); ok {
		return fmt.Sprint(claims[claimName])
	}
	return ""
}
