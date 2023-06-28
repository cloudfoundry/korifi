package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

type RequestValidator interface {
	DecodeAndValidateJSONPayload(r *http.Request, object any) error
	DecodeAndValidateURLValues(r *http.Request, payloadObject validation.KeyedPayload) error
	DecodeAndValidateYAMLPayload(r *http.Request, object any) error
}
