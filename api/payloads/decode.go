package payloads

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
)

type keyedPayload interface {
	SupportedKeys() []string
	DecodeFromURLValues(url.Values) error
}

func Decode(payloadObject keyedPayload, values url.Values) error {
	if err := checkKeysAreSupported(payloadObject, values); err != nil {
		return apierrors.NewUnknownKeyError(err, payloadObject.SupportedKeys())
	}
	if err := payloadObject.DecodeFromURLValues(values); err != nil {
		return apierrors.NewMessageParseError(err)
	}
	return nil
}

func checkKeysAreSupported(payloadObject keyedPayload, values url.Values) error {
	supportedKeys := map[string]bool{}
	for _, key := range payloadObject.SupportedKeys() {
		supportedKeys[key] = true
	}
	for key := range values {
		if !supportedKeys[key] {
			return fmt.Errorf("unsupported query parameter: %s", key)
		}
	}

	return nil
}
