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

	switch typedErr := err.(type) {
	case schema.MultiError:
		for _, v := range typedErr {
			if handledErr := handleSingleError(v, payloadObject); handledErr != nil {
				return handledErr
			}
		}

		return fmt.Errorf("unable to decode request query parameters: %w", err)
	default:
		return fmt.Errorf("unable to decode request query parameters: %w", err)
	}
}

func handleSingleError(err error, payloadObject keyedPayload) error {
	switch err.(type) {
	case schema.UnknownKeyError:
		return apierrors.NewUnknownKeyError(err, payloadObject.SupportedKeys())
	case schema.ConversionError:
		return apierrors.NewMessageParseError(err)
	default:
		return nil
	}
}
