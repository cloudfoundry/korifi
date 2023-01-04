package authorization

import (
	"encoding/base64"
	"errors"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
)

type InfoParser struct{}

func NewInfoParser() *InfoParser {
	return &InfoParser{}
}

func (p *InfoParser) Parse(authorizationHeader string) (Info, error) {
	if authorizationHeader == "" {
		return Info{}, apierrors.NewNotAuthenticatedError(errors.New("missing Authorization header"))
	}

	values := strings.Split(authorizationHeader, " ")
	if len(values) != 2 {
		return Info{}, apierrors.NewInvalidAuthError(errors.New("invalid Authorization header"))
	}

	scheme, data := values[0], values[1]
	switch strings.ToLower(scheme) {
	case BearerScheme:
		return Info{Token: data}, nil
	case CertScheme:
		certBytes, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return Info{}, apierrors.NewInvalidAuthError(err)
		}
		return Info{CertData: certBytes}, nil
	default:
		return Info{}, apierrors.NewInvalidAuthError(errors.New("unsupported authorization scheme"))
	}
}
