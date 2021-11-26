package authorization

import (
	"encoding/base64"
	"strings"
)

type InfoParser struct{}

func NewInfoParser() *InfoParser {
	return &InfoParser{}
}

func (p *InfoParser) Parse(authorizationHeader string) (Info, error) {
	if authorizationHeader == "" {
		return Info{}, NotAuthenticatedError{}
	}

	values := strings.Split(authorizationHeader, " ")
	if len(values) != 2 {
		return Info{}, InvalidAuthError{}
	}

	scheme, data := values[0], values[1]
	switch strings.ToLower(scheme) {
	case BearerScheme:
		return Info{Token: data}, nil
	case CertScheme:
		certBytes, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return Info{}, InvalidAuthError{}
		}
		return Info{CertData: certBytes}, nil
	default:
		return Info{}, InvalidAuthError{}
	}
}
