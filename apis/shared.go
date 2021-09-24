package apis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/cf-k8s-api/presenter"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

var Logger = ctrl.Log.WithName("Shared Handler Functions")

//counterfeiter:generate -o fake -fake-name ClientBuilder . ClientBuilder
type ClientBuilder func(*rest.Config) (client.Client, error)

type requestMalformedError struct {
	httpStatus    int
	errorResponse presenter.ErrorsResponse
}

func (rme *requestMalformedError) Error() string {
	return fmt.Sprintf("Error throwing an http %v", rme.httpStatus)
}

func DecodePayload(r *http.Request, object interface{}) *requestMalformedError {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&object)
	if err != nil {
		var unmarshalTypeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &unmarshalTypeError):
			Logger.Error(err, fmt.Sprintf("Request body contains an invalid value for the %q field (should be of type %v)", strings.Title(unmarshalTypeError.Field), unmarshalTypeError.Type))
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(fmt.Sprintf("%v must be a %v", strings.Title(unmarshalTypeError.Field), unmarshalTypeError.Type)),
			}
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			// check whether the message matches an "unknown field" error. If so, 422. Else, 400
			Logger.Error(err, fmt.Sprintf("Unknown field in JSON body: %T: %q", err, err.Error()))
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(fmt.Sprintf("invalid request body: %s", err.Error())),
			}
		default:
			Logger.Error(err, fmt.Sprintf("Unable to parse the JSON body: %T: %q", err, err.Error()))
			return &requestMalformedError{
				httpStatus:    http.StatusBadRequest,
				errorResponse: newMessageParseError(),
			}
		}
	}

	v := validator.New()

	// Register custom validators
	v.RegisterValidation("routepathstartswithslash", routePathStartsWithSlash)

	trans := registerDefaultTranslator(v)

	err = v.Struct(object)
	if err != nil {
		errorMap := err.(validator.ValidationErrors).Translate(trans)
		var errorMessages []string
		for _, msg := range errorMap {
			errorMessages = append(errorMessages, msg)
		}

		if len(errorMessages) > 0 {
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(strings.Join(errorMessages, ",")),
			}
		}
	}

	return nil
}

func registerDefaultTranslator(v *validator.Validate) ut.Translator {
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")
	en_translations.RegisterDefaultTranslations(v, trans)
	return trans
}

func newNotFoundError(resourceName string) presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-ResourceNotFound",
		Detail: fmt.Sprintf("%s not found", resourceName),
		Code:   10010,
	}}}
}

func newUnknownError() presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "UnknownError",
		Detail: "An unknown error occurred.",
		Code:   10001,
	}}}
}

func newMessageParseError() presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-MessageParseError",
		Detail: "Request invalid due to parse error: invalid request body",
		Code:   1001,
	}}}
}

func newUnprocessableEntityError(detail string) presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-UnprocessableEntity",
		Detail: detail,
		Code:   10008,
	}}}
}

func newUniquenessError(detail string) presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-UniquenessError",
		Detail: detail,
		Code:   10016,
	}}}
}

func writeNotFoundErrorResponse(w http.ResponseWriter, resourceName string) {
	responseBody, err := json.Marshal(newNotFoundError(resourceName))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(responseBody)
}

func writeUnknownErrorResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	responseBody, err := json.Marshal(newUnknownError())
	if err != nil {
		return
	}
	_, _ = w.Write(responseBody)
}

func writeErrorResponse(w http.ResponseWriter, rme *requestMalformedError) {
	w.WriteHeader(rme.httpStatus)
	responseBody, err := json.Marshal(rme.errorResponse)
	if err != nil {
		return
	}
	w.Write(responseBody)
}

func writeUnprocessableEntityError(w http.ResponseWriter, errorDetail string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	responseBody, err := json.Marshal(newUnprocessableEntityError(errorDetail))
	if err != nil {
		return
	}
	w.Write(responseBody)
}

func writeUniquenessError(w http.ResponseWriter, detail string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	responseBody, err := json.Marshal(newUniquenessError(detail))
	if err != nil {
		return
	}
	w.Write(responseBody)
}

// Custom field validators
func routePathStartsWithSlash(fl validator.FieldLevel) bool {
	if fl.Field().String() == "" {
		return true
	}

	if fl.Field().String()[0:1] != "/" {
		return false
	}

	return true
}
