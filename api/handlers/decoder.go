package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

type decoder interface {
	Decode(any) error
}

func BodyToObject[T any](r *http.Request, onlyKnownFields bool) (T, error) {
	var (
		out T
		d   decoder
		err error
	)

	defer r.Body.Close()

	d, err = newDecoder(r, onlyKnownFields)
	if err != nil {
		return out, err
	}

	err = translateError(d.Decode(&out))

	return out, err
}

func translateError(err error) error {
	if err != nil {
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {

		case errors.As(err, &unmarshalTypeError):
			titler := cases.Title(language.AmericanEnglish)
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("%v must be a %v", titler.String(unmarshalTypeError.Field), unmarshalTypeError.Type))

		case strings.Contains(err.Error(), "cannot unmarshal"):
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("incorrect data type: %s", err.Error()))

		case strings.Contains(err.Error(), "unknown field"):
			fallthrough
		case strings.Contains(err.Error(), "not found in type"):
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("invalid request body: %s", err.Error()))

		default:
			return apierrors.NewMessageParseError(err)
		}
	}

	return nil
}

func newDecoder(r *http.Request, onlyKnownFields bool) (decoder, error) {
	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case "":
		fallthrough
	case "application/json":
		dec := json.NewDecoder(r.Body)
		if onlyKnownFields {
			dec.DisallowUnknownFields()
		}
		return dec, nil
	case "application/x-yaml":
		fallthrough
	case "text/x-yaml":
		fallthrough
	case "text/yaml":
		dec := yaml.NewDecoder(r.Body)
		dec.KnownFields(onlyKnownFields)
		return dec, nil
	default:
		return nil, fmt.Errorf("unsupported Content-Type %q", contentType)
	}
}
