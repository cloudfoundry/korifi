package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"

	"github.com/jellydator/validation"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

type RequestValidator interface {
	DecodeAndValidateJSONPayload(r *http.Request, object interface{}) error
	DecodeAndValidateURLValues(r *http.Request, payloadObject KeyedPayload) error
}

type KeyedPayload interface {
	SupportedKeys() []string
	DecodeFromURLValues(url.Values) error
}

type IgnoredKeysPayload interface {
	IgnoredKeys() []*regexp.Regexp
}

type DecoderValidator struct{}

func NewDefaultDecoderValidator() DecoderValidator {
	return DecoderValidator{}
}

func (dv DecoderValidator) DecodeAndValidateJSONPayload(r *http.Request, object interface{}) error {
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.DisallowUnknownFields()
	err := decoder.Decode(object)
	if err != nil {
		var unmarshalTypeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &unmarshalTypeError):
			titler := cases.Title(language.AmericanEnglish)
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("%v must be a %v", titler.String(unmarshalTypeError.Field), unmarshalTypeError.Type))
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			// check whether the message matches an "unknown field" error. If so, 422. Else, 400
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("invalid request body: %s", err.Error()))
		default:
			return apierrors.NewMessageParseError(err)
		}
	}

	return dv.validatePayload(object)
}

func (dv DecoderValidator) DecodeAndValidateYAMLPayload(r *http.Request, object interface{}) error {
	decoder := yaml.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.KnownFields(false) // TODO: change this to true once we've added all manifest fields to payloads.Manifest
	err := decoder.Decode(object)
	if err != nil {
		return apierrors.NewMessageParseError(err)
	}

	return dv.validatePayload(object)
}

func (dv DecoderValidator) DecodeAndValidateURLValues(r *http.Request, object KeyedPayload) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	if err := checkKeysAreSupported(object, r.Form); err != nil {
		return apierrors.NewUnknownKeyError(err, object.SupportedKeys())
	}
	if err := object.DecodeFromURLValues(r.Form); err != nil {
		return apierrors.NewMessageParseError(err)
	}
	return dv.validatePayload(object)
}

func checkKeysAreSupported(payloadObject KeyedPayload, values url.Values) error {
	supportedKeys := map[string]bool{}
	for _, key := range payloadObject.SupportedKeys() {
		supportedKeys[key] = true
	}

	for key := range values {
		if !supportedKeys[key] && !isIgnored(payloadObject, key) {
			return fmt.Errorf("unsupported query parameter: %s", key)
		}
	}

	return nil
}

func isIgnored(payload KeyedPayload, key string) bool {
	ignoredKeysPayload, ok := payload.(IgnoredKeysPayload)
	if !ok {
		return false
	}

	for _, re := range ignoredKeysPayload.IgnoredKeys() {
		if re.MatchString(key) {
			return true
		}
	}

	return false
}

func (dv *DecoderValidator) validatePayload(object interface{}) error {
	t, ok := object.(validation.Validatable)
	if !ok {
		return nil
	}

	if err := t.Validate(); err != nil {
		return apierrors.NewUnprocessableEntityError(err, strings.Join(errorMessages(err), ", "))
	}

	return nil
}

func errorMessages(err error) []string {
	errs := prefixedErrorMessages("", err)
	sort.Strings(errs)
	return errs
}

var arrayIndexRegexp = regexp.MustCompile(`^\d+$`)

func prefixedErrorMessages(field string, err error) []string {
	errors, ok := err.(validation.Errors)
	if !ok {
		return []string{field + " " + err.Error()}
	}

	prefix := ""
	if field != "" {
		if arrayIndexRegexp.MatchString(field) {
			prefix = "[" + field + "]."
		} else {
			prefix = field + "."
		}
	}

	var messages []string
	for f, err := range errors {
		if arrayIndexRegexp.MatchString(f) {
			prefix = strings.TrimSuffix(prefix, ".")
		}

		ems := prefixedErrorMessages(f, err)
		for _, e := range ems {
			messages = append(messages, prefix+e)
		}
	}

	return messages
}
