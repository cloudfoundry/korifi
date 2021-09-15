package apis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/go-playground/validator/v10"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-api/presenter"
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

func DecodePayload(r *http.Request, object interface{}) error {

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
		default:
			Logger.Error(err, "Unable to parse the JSON body")
			return &requestMalformedError{
				httpStatus:    http.StatusBadRequest,
				errorResponse: newMessageParseError(),
			}
		}
	}

	v := validator.New()
	err = v.Struct(object)

	if err != nil {
		var errorMessages []string
		for _, e := range err.(validator.ValidationErrors) {
			errorMessages = append(errorMessages, fmt.Sprintf("%v must be a %v", strings.Title(e.Field()), e.Type()))
		}

		if len(errorMessages) > 0 {
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(strings.Join(errorMessages[:], ",")),
			}
		}
	}

	return nil
}

func newNotFoundError(resourceName string) presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  fmt.Sprintf("%s not found", resourceName),
		Detail: "CF-ResourceNotFound",
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
	w.WriteHeader(http.StatusNotFound)
	responseBody, err := json.Marshal(newNotFoundError(resourceName))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
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
		w.WriteHeader(rme.httpStatus)
		return
	}
	w.Write(responseBody)
}

func writeMessageParseError(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	responseBody, err := json.Marshal(newMessageParseError())
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Write(responseBody)
}

func writeUnprocessableEntityError(w http.ResponseWriter, errorDetail string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	responseBody, err := json.Marshal(newUnprocessableEntityError(errorDetail))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	w.Write(responseBody)
}

func writeUniquenessError(w http.ResponseWriter, detail string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	responseBody, err := json.Marshal(newUniquenessError(detail))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	w.Write(responseBody)
}
