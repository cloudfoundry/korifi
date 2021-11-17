package apis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"code.cloudfoundry.org/bytefmt"
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

func decodeAndValidateJSONPayload(r *http.Request, object interface{}) *requestMalformedError {
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	decoder.DisallowUnknownFields()
	err := decoder.Decode(object)
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

	return validatePayload(object)
}

func validatePayload(object interface{}) *requestMalformedError {
	v := validator.New()

	trans := registerDefaultTranslator(v)

	// Register custom validators
	v.RegisterValidation("routepathstartswithslash", routePathStartsWithSlash)
	v.RegisterValidation("megabytestring", megabyteFormattedString, true)

	v.RegisterStructValidation(checkRoleTypeAndOrgSpace, payloads.RoleCreate{})
	v.RegisterTranslation("cannot_have_both_org_and_space_set", trans, func(ut ut.Translator) error {
		return ut.Add("cannot_have_both_org_and_space_set", "Cannot pass both 'organization' and 'space' in a create role request", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("cannot_have_both_org_and_space_set", fe.Field())
		return t
	})
	v.RegisterTranslation("valid_role", trans, func(ut ut.Translator) error {
		return ut.Add("valid_role", "{0} is not a valid role", false)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("valid_role", fmt.Sprintf("%v", fe.Value()))
		return t
	})

	err := v.Struct(object)
	if err != nil {
		switch typedErr := err.(type) {
		case validator.ValidationErrors:
			errorMap := typedErr.Translate(trans)
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
		default:
			return &requestMalformedError{
				httpStatus:    http.StatusUnprocessableEntity,
				errorResponse: newUnprocessableEntityError(err.Error()),
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

func newUnauthenticatedError() presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-NotAuthenticated",
		Detail: "No auth token was given, but authentication is required for this endpoint",
		Code:   10002,
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

func newInvalidRequestError(detail string) presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-InvalidRequest",
		Detail: detail,
		Code:   10004,
	}}}
}

func newPackageBitsAlreadyUploadedError() presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-PackageBitsAlreadyUploaded",
		Detail: "Bits may be uploaded only once. Create a new package to upload different bits.",
		Code:   150004,
	}}}
}

func newUnknownKeyError() presenter.ErrorsResponse {
	return presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
		Title:  "CF-BadQueryParameter",
		Detail: "The query parameter is invalid: Valid parameters are: 'names'",
		Code:   10005,
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

func writeUnauthorizedErrorResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	responseBody, err := json.Marshal(newUnauthenticatedError())
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

func writeInvalidRequestError(w http.ResponseWriter, detail string) {
	w.WriteHeader(http.StatusBadRequest)

	responseBody, err := json.Marshal(newInvalidRequestError(detail))
	if err != nil {
		return
	}
	w.Write(responseBody)
}

func writePackageBitsAlreadyUploadedError(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)

	responseBody, err := json.Marshal(newPackageBitsAlreadyUploadedError())
	if err != nil {
		return
	}
	w.Write(responseBody)
}

func writeUnknownKeyError(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	responseBody, err := json.Marshal(newUnknownKeyError())
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

func checkRoleTypeAndOrgSpace(sl validator.StructLevel) {
	roleCreate := sl.Current().Interface().(payloads.RoleCreate)

	if roleCreate.Relationships.Organization != nil && roleCreate.Relationships.Space != nil {
		sl.ReportError(roleCreate.Relationships.Organization, "relationships.organization", "Organization", "cannot_have_both_org_and_space_set", "")
	}

	roleType := RoleName(roleCreate.Type)

	switch roleType {
	case RoleSpaceManager:
		fallthrough
	case RoleSpaceAuditor:
		fallthrough
	case RoleSpaceDeveloper:
		fallthrough
	case RoleSpaceSupporter:
		if roleCreate.Relationships.Space == nil {
			sl.ReportError(roleCreate.Relationships.Space, "relationships.space", "Space", "required", "")
		}
	case RoleOrganizationUser:
		fallthrough
	case RoleOrganizationAuditor:
		fallthrough
	case RoleOrganizationManager:
		fallthrough
	case RoleOrganizationBillingManager:
		if roleCreate.Relationships.Organization == nil {
			sl.ReportError(roleCreate.Relationships.Organization, "relationships.organization", "Organization", "required", "")
		}

	case RoleName(""):
	default:
		sl.ReportError(roleCreate.Type, "type", "Role type", "valid_role", "")
	}
}

func megabyteFormattedString(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return true
	}

	_, err := bytefmt.ToMegabytes(val)
	return err == nil
}
