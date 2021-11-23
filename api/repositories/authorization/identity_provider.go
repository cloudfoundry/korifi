package authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	bearerScheme string = "bearer"
	certScheme   string = "clientcert"
)

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
	certInspector  IdentityInspector
}

func NewIdentityProvider(tokenInspector, certInspector IdentityInspector) *IdentityProvider {
	return &IdentityProvider{
		tokenInspector: tokenInspector,
		certInspector:  certInspector,
	}
}

func (p *IdentityProvider) GetIdentity(ctx context.Context, authorizationHeader string) (Identity, error) {
	if authorizationHeader == "" {
		return Identity{}, InvalidAuthError{}
	}

	scheme, value, err := parseAuthorizationHeader(authorizationHeader)
	if err != nil {
		return Identity{}, err
	}

	switch strings.ToLower(scheme) {
	case bearerScheme:
		return p.tokenInspector.WhoAmI(ctx, value)
	case certScheme:
		return p.certInspector.WhoAmI(ctx, value)
	default:
		return Identity{}, fmt.Errorf("unsupported authentication scheme %q", scheme)
	}
}

func parseAuthorizationHeader(headerValue string) (string, string, error) {
	values := strings.Split(headerValue, " ")
	if len(values) != 2 {
		return "", "", errors.New("failed to parse authorization header")
	}
	return values[0], values[1], nil
}
