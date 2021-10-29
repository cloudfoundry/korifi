package authorization

import (
	"context"
	"errors"
	"strings"
)

const bearerScheme string = "bearer"

//counterfeiter:generate -o fake -fake-name IdentityInspector . IdentityInspector

type Identity struct {
	Name string
	Kind string
}

type IdentityInspector interface {
	WhoAmI(context.Context, string) (Identity, error)
}

type IdentityProvider struct {
	tokenInspector IdentityInspector
}

func NewIdentityProvider(tokenInspector IdentityInspector) *IdentityProvider {
	return &IdentityProvider{
		tokenInspector: tokenInspector,
	}
}

func (p *IdentityProvider) GetIdentity(ctx context.Context, authorizationHeader string) (Identity, error) {
	if authorizationHeader == "" {
		return Identity{}, UnauthorizedErr{}
	}

	scheme, value, err := parseAuthorizationHeader(authorizationHeader)
	if err != nil {
		return Identity{}, err
	}

	switch strings.ToLower(scheme) {
	case bearerScheme:
		return p.tokenInspector.WhoAmI(ctx, value)
	default:
		return Identity{}, errors.New("unsupported authentication scheme")
	}
}

func parseAuthorizationHeader(headerValue string) (string, string, error) {
	values := strings.Split(headerValue, " ")
	if len(values) != 2 {
		return "", "", errors.New("failed to parse authorization header")
	}
	return values[0], values[1], nil
}
