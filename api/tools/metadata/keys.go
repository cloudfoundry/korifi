package metadata

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func CloudfoundryKeyCheck(key any) error {
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("expected string key, got %T", key)
	}

	u, err := url.ParseRequestURI("https://" + keyStr) // without the scheme, the hostname will be parsed as a path
	if err != nil {
		return nil
	}

	if strings.HasSuffix(u.Hostname(), "cloudfoundry.org") {
		return errors.New("label/annotation key cannot use the cloudfoundry.org domain")
	}
	return nil
}
