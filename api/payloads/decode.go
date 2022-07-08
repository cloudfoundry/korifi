package payloads

import (
	"fmt"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"github.com/gorilla/schema"
)

type keyedPayload interface {
	SupportedKeys() []string
}

func Decode(payloadObject keyedPayload, src map[string][]string) error {
	err := schema.NewDecoder().Decode(payloadObject, src)
	if err == nil {
		return nil
	}

	switch err.(type) {
	case schema.MultiError:
		multiError := err.(schema.MultiError)
		for _, v := range multiError {
			_, ok := v.(schema.UnknownKeyError)
			if ok {
				return apierrors.NewUnknownKeyError(err, payloadObject.SupportedKeys())
			}
		}

		return fmt.Errorf("unable to decode request query parameters: %w", err)
	default:
		return fmt.Errorf("unable to decode request query parameters: %w", err)
	}
}
